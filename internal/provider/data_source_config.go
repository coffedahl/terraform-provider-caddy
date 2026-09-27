// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var (
	_ datasource.DataSource              = &configDataSource{}
	_ datasource.DataSourceWithConfigure = &configDataSource{}
)

func NewConfigDataSource() datasource.DataSource { return &configDataSource{} }

type configDataSource struct {
	client *client.Client
}

type configDataSourceModel struct {
	Path types.String `tfsdk:"path"`
	JSON types.String `tfsdk:"json"`
	ID   types.String `tfsdk:"id"`
}

func (d *configDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (d *configDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Read a slice of the live Caddy JSON config. Useful for debugging and drift inspection.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Same as `path`.",
			},
			"path": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Admin API path, default `/config/`.",
			},
			"json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Pretty-printed JSON at that path.",
			},
		},
	}
}

func (d *configDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromData(req, resp)
}

func (d *configDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data configDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	path := data.Path.ValueString()
	if path == "" {
		path = "/config/"
	}
	raw, _, err := d.client.Get(ctx, path)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_config", err.Error())
		return
	}
	var pretty any
	if err := json.Unmarshal(raw, &pretty); err != nil {
		data.JSON = types.StringValue(string(raw))
	} else {
		data.JSON = types.StringValue(prettyJSON(pretty))
	}
	data.Path = types.StringValue(path)
	data.ID = types.StringValue(path)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
