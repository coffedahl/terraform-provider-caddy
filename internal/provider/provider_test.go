// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"caddy": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	t.Setenv("TF_ACC_PROVIDER_NAMESPACE", "coffedahl")
	t.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	if os.Getenv("TF_ACC_TERRAFORM_PATH") == "" {
		if _, err := os.Stat("/usr/bin/tofu"); err == nil {
			t.Setenv("TF_ACC_TERRAFORM_PATH", "/usr/bin/tofu")
		}
	}
	endpoint := os.Getenv("CADDY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:2019"
	}
	c, err := client.New(endpoint, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// TF_ACC is set whenever this runs, so an unreachable Caddy is a failure,
	// not a skip: a skipped acceptance run would look green in CI.
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Caddy Admin API not reachable at %s: %v", endpoint, err)
	}
}

const testAccProviderConfig = `
terraform {
  required_providers {
    caddy = {
      source = "coffedahl/caddy"
    }
  }
}

provider "caddy" {}
`

func TestAccServerAndSite(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `

resource "caddy_server" "http" {
  name   = "tfacc"
  listen = [":2015"]
  disable_auto_https = true
}

resource "caddy_site" "hello" {
  name      = "tfacc-hello"
  server_id = caddy_server.http.id
  hosts     = ["hello.localhost"]

  handle {
    respond {
      body        = "hello-from-tofu"
      status_code = 200
    }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("caddy_server.http", "name", "tfacc"),
					resource.TestCheckResourceAttr("caddy_site.hello", "hosts.0", "hello.localhost"),
				),
			},
			{
				Config: testAccProviderConfig + `
resource "caddy_server" "http" {
  name   = "tfacc"
  listen = [":2015"]
  disable_auto_https = true
}

resource "caddy_site" "apps" {
  name      = "tfacc-wildcard"
  server_id = caddy_server.http.id
  hosts     = ["*.localhost"]
  priority  = 1

  handle {
    match {
      host = ["api.localhost"]
      path = ["/v1/*"]
    }
    reverse_proxy {
      upstream {
        dial = "127.0.0.1:18080"
      }
    }
  }

  handle {
    match {
      path = ["/static/*"]
    }
    file_server {
      root = "/tmp"
    }
  }

  handle {
    abort = true
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("caddy_site.apps", "hosts.0", "*.localhost"),
					resource.TestCheckResourceAttr("caddy_site.apps", "handle.#", "3"),
				),
			},
			{
				ResourceName:            "caddy_server.http",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"protocols"},
			},
			{
				ResourceName:            "caddy_site.apps",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"handle.0.name", "handle.1.name", "handle.2.name", "priority"},
			},
		},
	})
}

func TestAccImportByIndex(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "caddy_server" "http" {
  name   = "tfacc-idx"
  listen = [":2016"]
  disable_auto_https = true
}

resource "caddy_site" "hello" {
  name      = "tfacc-idx-hello"
  server_id = caddy_server.http.id
  hosts     = ["idx.localhost"]

  handle {
    respond {
      body        = "idx"
      status_code = 200
    }
  }
}
`,
			},
			{
				ResourceName:      "caddy_site.hello",
				ImportState:       true,
				ImportStateId:     "tfacc-idx/0",
				ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{
					"handle.0.name",
				},
			},
		},
	})
}

func TestAccInventoryDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testAccProviderConfig + `
resource "caddy_server" "http" {
  name   = "tfacc-inv"
  listen = [":2017"]
  disable_auto_https = true
}

resource "caddy_site" "hello" {
  name      = "tfacc-inv-hello"
  server_id = caddy_server.http.id
  hosts     = ["inv.localhost"]

  handle {
    respond {
      body = "inv"
    }
  }
}

data "caddy_inventory" "live" {
  depends_on = [caddy_site.hello]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.caddy_inventory.live", "import_commands.#"),
				),
			},
		},
	})
}
