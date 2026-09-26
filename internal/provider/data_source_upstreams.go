// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var (
	_ datasource.DataSource              = &upstreamsDataSource{}
	_ datasource.DataSourceWithConfigure = &upstreamsDataSource{}
)

func NewUpstreamsDataSource() datasource.DataSource { return &upstreamsDataSource{} }

type upstreamsDataSource struct {
	client *client.Client
}

type upstreamsModel struct {
	ID   types.String `tfsdk:"id"`
	JSON types.String `tfsdk:"json"`
}

func (d *upstreamsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_upstream_status"
}

func (d *upstreamsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Live reverse_proxy upstream status from `GET /reverse_proxy/upstreams`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"json": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "JSON array of upstreams with `address`, `num_requests`, and `fails`.",
			},
		},
	}
}

func (d *upstreamsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromData(req, resp)
}

func (d *upstreamsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data upstreamsModel
	raw, _, err := d.client.Get(ctx, "/reverse_proxy/upstreams")
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_upstream_status", err.Error())
		return
	}
	data.JSON = types.StringValue(string(raw))
	data.ID = types.StringValue("reverse_proxy/upstreams")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
