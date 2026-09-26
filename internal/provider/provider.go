// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var _ provider.Provider = &caddyProvider{}

type caddyProvider struct {
	version string
}

type caddyProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *caddyProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "caddy"
	resp.Version = p.version
}

func (p *caddyProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage a running [Caddy](https://caddyserver.com/) 2 instance through its Admin API. " +
			"The provider compiles HCL into Caddy's native JSON config and uses `@id` for resource identity.\n\n" +
			"Prefer a unix socket admin endpoint in production. Wildcard certificates require the ACME DNS-01 " +
			"challenge, which means Caddy must be built with a [caddy-dns](https://github.com/caddy-dns) module.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Caddy Admin API address. Accepts `http://127.0.0.1:2019` (default), " +
					"`https://...` for remote admin, or `unix:///run/caddy/admin.sock`. " +
					"May also be set with `CADDY_ENDPOINT` or `CADDY_ADMIN`.",
			},
		},
	}
}

func (p *caddyProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data caddyProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := os.Getenv("CADDY_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("CADDY_ADMIN")
	}
	if !data.Endpoint.IsNull() && !data.Endpoint.IsUnknown() {
		endpoint = data.Endpoint.ValueString()
	}
	if endpoint == "" {
		endpoint = "http://127.0.0.1:2019"
	}

	c, err := client.New(endpoint, 30*time.Second)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Invalid Caddy endpoint",
			err.Error(),
		)
		return
	}

	if err := c.Ping(ctx); err != nil {
		resp.Diagnostics.AddError(
			"Unable to reach Caddy Admin API",
			"Could not GET /config/ at "+endpoint+": "+err.Error(),
		)
		return
	}

	if err := c.EnsurePersist(ctx); err != nil {
		resp.Diagnostics.AddWarning(
			"Could not enable Caddy config persistence",
			err.Error()+"\nAPI changes are stored in Caddy's autosave.json only if persist is on. "+
				"Start Caddy with `caddy run --resume` (not a Caddyfile reload) or API routes disappear on restart.",
		)
	}

	resp.DataSourceData = c
	resp.ResourceData = c
}

func (p *caddyProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewServerResource,
		NewSiteResource,
		NewHandleResource,
		NewTLSPolicyResource,
	}
}

func (p *caddyProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewConfigDataSource,
		NewUpstreamsDataSource,
		NewInventoryDataSource,
	}
}

// New returns a provider factory. version is injected by GoReleaser.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &caddyProvider{version: version}
	}
}

func clientFrom(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "expected *client.Client")
		return nil
	}
	return c
}

func clientFromData(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *client.Client {
	if req.ProviderData == nil {
		return nil
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", "expected *client.Client")
		return nil
	}
	return c
}
