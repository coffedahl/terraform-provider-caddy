// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var (
	_ datasource.DataSource              = &inventoryDataSource{}
	_ datasource.DataSourceWithConfigure = &inventoryDataSource{}
)

func NewInventoryDataSource() datasource.DataSource { return &inventoryDataSource{} }

type inventoryDataSource struct {
	client *client.Client
}

type inventoryModel struct {
	ID             types.String `tfsdk:"id"`
	ImportCommands types.List   `tfsdk:"import_commands"`
	Servers        types.List   `tfsdk:"servers"`
	TLSPolicies    types.List   `tfsdk:"tls_policies"`
}

func (d *inventoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_inventory"
}

func (d *inventoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	routeType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"index":        types.Int64Type,
		"id":           types.StringType,
		"hosts":        types.ListType{ElemType: types.StringType},
		"handle_count": types.Int64Type,
		"import_id":    types.StringType,
	}}
	serverType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":   types.StringType,
		"listen": types.ListType{ElemType: types.StringType},
		"routes": types.ListType{ElemType: routeType},
	}}
	policyType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"index":     types.Int64Type,
		"id":        types.StringType,
		"subjects":  types.ListType{ElemType: types.StringType},
		"on_demand": types.BoolType,
		"import_id": types.StringType,
	}}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Snapshot of a live Caddy instance for `tofu import`. " +
			"`import_commands` are ready-to-run lines. Routes without `@id` are addressed as `{server}/{index}`; " +
			"importing a site stamps an `@id` onto the live route so later applies can find it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"import_commands": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Suggested `tofu import ...` commands for every server, site, and TLS policy.",
			},
			"servers": schema.ListAttribute{
				Computed:            true,
				ElementType:         serverType,
				MarkdownDescription: "HTTP servers and their top-level routes.",
			},
			"tls_policies": schema.ListAttribute{
				Computed:    true,
				ElementType: policyType,
			},
		},
	}
}

func (d *inventoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromData(req, resp)
}

func (d *inventoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	inv, err := d.snapshot(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_inventory", err.Error())
		return
	}

	var data inventoryModel
	data.ID = types.StringValue("inventory")
	cmds, diags := listValue(ctx, caddyjson.ImportCommands(inv))
	resp.Diagnostics.Append(diags...)
	data.ImportCommands = cmds

	servers, diags := inventoryServersValue(ctx, inv.Servers)
	resp.Diagnostics.Append(diags...)
	data.Servers = servers

	policies, diags := inventoryPoliciesValue(ctx, inv.Policies)
	resp.Diagnostics.Append(diags...)
	data.TLSPolicies = policies

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *inventoryDataSource) snapshot(ctx context.Context) (caddyjson.Inventory, error) {
	live, _, err := loadServers(ctx, d.client)
	if err != nil {
		return caddyjson.Inventory{}, err
	}
	raw, _, err := d.client.Get(ctx, tlsPoliciesPath())
	var policies []caddyjson.LivePolicy
	if err == nil {
		policies, err = caddyjson.ParsePolicies(raw)
		if err != nil {
			return caddyjson.Inventory{}, err
		}
	} else if !client.IsMissing(err) {
		return caddyjson.Inventory{}, err
	}
	return caddyjson.Inventory{Servers: live, Policies: policies}, nil
}

func inventoryServersValue(ctx context.Context, servers []caddyjson.LiveServer) (types.List, diag.Diagnostics) {
	routeType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"index":        types.Int64Type,
		"id":           types.StringType,
		"hosts":        types.ListType{ElemType: types.StringType},
		"handle_count": types.Int64Type,
		"import_id":    types.StringType,
	}}
	serverType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":   types.StringType,
		"listen": types.ListType{ElemType: types.StringType},
		"routes": types.ListType{ElemType: routeType},
	}}
	if len(servers) == 0 {
		return types.ListValueMust(serverType, []attr.Value{}), nil
	}

	var diags diag.Diagnostics
	vals := make([]attr.Value, 0, len(servers))
	for _, srv := range servers {
		listen, d := types.ListValueFrom(ctx, types.StringType, srv.Listen)
		diags.Append(d...)
		routes := make([]attr.Value, 0, len(srv.Routes))
		for _, rt := range srv.Routes {
			hosts, d := types.ListValueFrom(ctx, types.StringType, rt.Hosts)
			diags.Append(d...)
			importID := srv.Name + "/" + strconv.Itoa(rt.Index)
			if rt.ID != "" {
				importID = srv.Name + "/" + rt.ID
			}
			obj, d := types.ObjectValue(routeType.AttrTypes, map[string]attr.Value{
				"index":        types.Int64Value(int64(rt.Index)),
				"id":           types.StringValue(rt.ID),
				"hosts":        hosts,
				"handle_count": types.Int64Value(int64(len(rt.Handles))),
				"import_id":    types.StringValue(importID),
			})
			diags.Append(d...)
			routes = append(routes, obj)
		}
		routeList, d := types.ListValue(routeType, routes)
		diags.Append(d...)
		obj, d := types.ObjectValue(serverType.AttrTypes, map[string]attr.Value{
			"name":   types.StringValue(srv.Name),
			"listen": listen,
			"routes": routeList,
		})
		diags.Append(d...)
		vals = append(vals, obj)
	}
	list, d := types.ListValue(serverType, vals)
	diags.Append(d...)
	return list, diags
}

func inventoryPoliciesValue(ctx context.Context, policies []caddyjson.LivePolicy) (types.List, diag.Diagnostics) {
	policyType := types.ObjectType{AttrTypes: map[string]attr.Type{
		"index":     types.Int64Type,
		"id":        types.StringType,
		"subjects":  types.ListType{ElemType: types.StringType},
		"on_demand": types.BoolType,
		"import_id": types.StringType,
	}}
	if len(policies) == 0 {
		return types.ListValueMust(policyType, []attr.Value{}), nil
	}
	var diags diag.Diagnostics
	vals := make([]attr.Value, 0, len(policies))
	for _, p := range policies {
		subjects, d := types.ListValueFrom(ctx, types.StringType, p.Subjects)
		diags.Append(d...)
		importID := strconv.Itoa(p.Index)
		if p.ID != "" {
			importID = p.ID
		}
		obj, d := types.ObjectValue(policyType.AttrTypes, map[string]attr.Value{
			"index":     types.Int64Value(int64(p.Index)),
			"id":        types.StringValue(p.ID),
			"subjects":  subjects,
			"on_demand": types.BoolValue(p.OnDemand),
			"import_id": types.StringValue(importID),
		})
		diags.Append(d...)
		vals = append(vals, obj)
	}
	list, d := types.ListValue(policyType, vals)
	diags.Append(d...)
	return list, diags
}
