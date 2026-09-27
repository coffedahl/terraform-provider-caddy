// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

func testAccClient(t *testing.T) *client.Client {
	t.Helper()
	endpoint := os.Getenv("CADDY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:2019"
	}
	c, err := client.New(endpoint, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// testAccRouteIDs returns the top-level route @ids of server.
func testAccRouteIDs(t *testing.T, server string) []string {
	t.Helper()
	raw, _, err := testAccClient(t).Get(context.Background(), serverRoutesPath(server))
	if client.IsMissing(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	routes, err := caddyjson.DecodeObjectList(raw)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(routes))
	for _, r := range routes {
		id, _ := r["@id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func testAccPolicyIDs(t *testing.T) []string {
	t.Helper()
	list, err := loadPolicies(context.Background(), testAccClient(t))
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(list))
	for _, p := range list {
		id, _ := p["@id"].(string)
		ids = append(ids, id)
	}
	return ids
}

func testAccCheckRoutes(t *testing.T, server string, want ...string) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		got := testAccRouteIDs(t, server)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("server %q routes = %v, want %v", server, got, want)
		}
		return nil
	}
}

func TestAccSitePriorityBeyondRouteCount(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "caddy_server" "http" {
  name               = "tfacc-prio"
  listen             = [":2020"]
  disable_auto_https = true
}

resource "caddy_site" "only" {
  name      = "tfacc-prio-only"
  server_id = caddy_server.http.id
  hosts     = ["prio.localhost"]
  priority  = 5

  handle {
    respond {
      body = "prio"
    }
  }
}
`,
				Check: resource.TestCheckResourceAttr("caddy_site.only", "priority", "5"),
			},
		},
	})
}

func TestAccSiteMoveServer(t *testing.T) {
	cfg := func(server string) string {
		return testAccProviderConfig + fmt.Sprintf(`
resource "caddy_server" "a" {
  name               = "tfacc-move-a"
  listen             = [":2021"]
  disable_auto_https = true
}

resource "caddy_server" "b" {
  name               = "tfacc-move-b"
  listen             = [":2022"]
  disable_auto_https = true
}

resource "caddy_site" "s" {
  name      = "tfacc-move-site"
  server_id = caddy_server.%s.id
  hosts     = ["move.localhost"]

  handle {
    respond {
      body = "move"
    }
  }
}
`, server)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg("a"),
				Check:  testAccCheckRoutes(t, "tfacc-move-a", "tfacc-move-site"),
			},
			{
				Config: cfg("b"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckRoutes(t, "tfacc-move-a"),
					testAccCheckRoutes(t, "tfacc-move-b", "tfacc-move-site"),
				),
			},
		},
	})
}

func TestAccHandle(t *testing.T) {
	cfg := func(body string) string {
		return testAccProviderConfig + fmt.Sprintf(`
resource "caddy_server" "http" {
  name               = "tfacc-handle"
  listen             = [":2023"]
  disable_auto_https = true
}

resource "caddy_site" "apps" {
  name      = "tfacc-handle-site"
  server_id = caddy_server.http.id
  hosts     = ["*.localhost"]

  handle {
    respond {
      body = "fallback"
    }
  }
}

resource "caddy_handle" "api" {
  name     = "tfacc-handle-api"
  site_id  = caddy_site.apps.id
  priority = 0

  match {
    host = ["api.localhost"]
  }

  respond {
    body        = %q
    status_code = 200
  }
}
`, body)
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg("v1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("caddy_handle.api", "id", "tfacc-handle-api"),
					resource.TestCheckResourceAttr("caddy_handle.api", "respond.0.body", "v1"),
					resource.TestCheckResourceAttr("caddy_site.apps", "handle.#", "1"),
				),
			},
			{
				Config: cfg("v2"),
				Check:  resource.TestCheckResourceAttr("caddy_handle.api", "respond.0.body", "v2"),
			},
			{
				ResourceName:      "caddy_handle.api",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccTLSPolicyOrdering(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "caddy_server" "http" {
  name               = "tfacc-tls"
  listen             = [":2024"]
  disable_auto_https = true
}

resource "caddy_tls_policy" "default" {
  name = "tfacc-tls-default"

  issuer {
    module = "internal"
  }
}

resource "caddy_site" "internal" {
  name      = "tfacc-tls-site"
  server_id = caddy_server.http.id
  hosts     = ["tls.localhost"]

  tls {
    internal = true
  }

  handle {
    respond {
      body = "tls"
    }
  }

  depends_on = [caddy_tls_policy.default]
}
`,
				Check: func(_ *terraform.State) error {
					ids := testAccPolicyIDs(t)
					if len(ids) == 0 || ids[len(ids)-1] != "tfacc-tls-default" {
						return fmt.Errorf("catch-all policy must be last, got order %v", ids)
					}
					return nil
				},
			},
			{
				ResourceName:      "caddy_tls_policy.default",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccOnDemandConfig(ask string) string {
	askLine := ""
	if ask != "" {
		askLine = fmt.Sprintf("ask = %q", ask)
	}
	return testAccProviderConfig + fmt.Sprintf(`
resource "caddy_tls_policy" "od" {
  name      = "tfacc-ondemand"
  subjects  = ["od.localhost"]
  on_demand = true
  %s

  issuer {
    module = "internal"
  }
}
`, askLine)
}

func TestAccTLSPolicyOnDemand(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		CheckDestroy: func(_ *terraform.State) error {
			raw, _, err := testAccClient(t).Get(context.Background(), "/config/apps/tls/automation/on_demand")
			if client.IsMissing(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if s := string(raw); s != "null\n" && s != "null" {
				return fmt.Errorf("on_demand config left behind after destroy: %s", s)
			}
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: testAccOnDemandConfig("http://127.0.0.1:5555/allow"),
				Check:  resource.TestCheckResourceAttr("caddy_tls_policy.od", "ask", "http://127.0.0.1:5555/allow"),
			},
			{
				Config: testAccOnDemandConfig("http://127.0.0.1:5555/v2"),
				Check:  resource.TestCheckResourceAttr("caddy_tls_policy.od", "ask", "http://127.0.0.1:5555/v2"),
			},
			{
				Config: testAccOnDemandConfig(""),
				Check:  resource.TestCheckNoResourceAttr("caddy_tls_policy.od", "ask"),
			},
		},
	})
}

func TestAccServerExistingNotClobbered(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					c := testAccClient(t)
					ctx := context.Background()
					if err := c.EnsureHTTPApp(ctx); err != nil {
						t.Fatal(err)
					}
					_ = c.Delete(ctx, "/config/apps/http/servers/tfacc-existing")
					if err := c.Put(ctx, "/config/apps/http/servers/tfacc-existing", map[string]any{
						"listen": []string{":2025"},
						"routes": []map[string]any{{"@id": "hand-made", "handle": []map[string]any{{"handler": "static_response", "body": "keep me"}}}},
					}); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccProviderConfig + `
resource "caddy_server" "existing" {
  name   = "tfacc-existing"
  listen = [":2025"]
}
`,
				ExpectError: regexpMustCompile(`(?i)already exists|import`),
			},
		},
	})
	// Clean up the hand-made server outside Terraform's knowledge.
	_ = testAccClient(t).Delete(context.Background(), "/config/apps/http/servers/tfacc-existing")
	if ids := testAccRouteIDs(t, "tfacc-existing"); len(ids) != 0 {
		t.Fatalf("cleanup failed: %v", ids)
	}
}

func TestAccDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
data "caddy_config" "admin" {
  path = "/config/"
}

data "caddy_upstream_status" "live" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.caddy_config.admin", "json"),
					resource.TestCheckResourceAttrSet("data.caddy_upstream_status.live", "json"),
				),
			},
		},
	})
}

func regexpMustCompile(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// TestAccSiteCertificateFile needs a PEM pair that Caddy itself can read, so
// it only runs when CADDY_TEST_CERT_DIR names such a directory (as seen by
// Caddy, which may be inside a container).
func TestAccSiteCertificateFile(t *testing.T) {
	dir := os.Getenv("CADDY_TEST_CERT_DIR")
	if dir == "" {
		t.Skip("CADDY_TEST_CERT_DIR not set")
	}
	cfg := func(withTLS bool) string {
		tls := ""
		if withTLS {
			tls = fmt.Sprintf(`
  tls {
    certificate_file = "%[1]s/cert.pem"
    key_file         = "%[1]s/key.pem"
  }`, dir)
		}
		return testAccProviderConfig + fmt.Sprintf(`
resource "caddy_server" "https" {
  name   = "tfacc-cert"
  listen = [":2026"]
}

resource "caddy_site" "cert" {
  name      = "tfacc-cert-site"
  server_id = caddy_server.https.id
  hosts     = ["cert.localhost"]
%s

  handle {
    respond {
      body = "cert"
    }
  }
}
`, tls)
	}
	loadFiles := func(want int) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			raw, _, err := testAccClient(t).Get(context.Background(), loadFilesPath)
			if client.IsMissing(err) {
				raw = []byte("[]")
			} else if err != nil {
				return err
			}
			list, err := caddyjson.DecodeObjectList(raw)
			if err != nil {
				return err
			}
			if len(list) != want {
				return fmt.Errorf("load_files has %d entries, want %d", len(list), want)
			}
			return nil
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: cfg(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("caddy_site.cert", "tls.0.certificate_file", dir+"/cert.pem"),
					loadFiles(1),
				),
			},
			{
				Config: cfg(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("caddy_site.cert", "tls.#", "0"),
					loadFiles(0),
				),
			},
		},
	})
}
