// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var (
	_ resource.Resource                = &handleResource{}
	_ resource.ResourceWithConfigure   = &handleResource{}
	_ resource.ResourceWithImportState = &handleResource{}
)

func NewHandleResource() resource.Resource { return &handleResource{} }

type handleResource struct {
	client *client.Client
}

type handleResourceModel struct {
	ID           types.String        `tfsdk:"id"`
	Name         types.String        `tfsdk:"name"`
	SiteID       types.String        `tfsdk:"site_id"`
	Priority     types.Int64         `tfsdk:"priority"`
	Match        []matchModel        `tfsdk:"match"`
	ReverseProxy []reverseProxyModel `tfsdk:"reverse_proxy"`
	FileServer   []fileServerModel   `tfsdk:"file_server"`
	Respond      []respondModel      `tfsdk:"respond"`
	Redir        []redirModel        `tfsdk:"redir"`
	Rewrite      []rewriteModel      `tfsdk:"rewrite"`
	Header       []headerModel       `tfsdk:"header"`
	Encode       []encodeModel       `tfsdk:"encode"`
	Abort        types.Bool          `tfsdk:"abort"`
	RawJSON      types.String        `tfsdk:"raw_json"`
}

func (m handleResourceModel) asHandle() handleModel {
	return handleModel{
		Name:         m.Name,
		Priority:     m.Priority,
		Match:        m.Match,
		ReverseProxy: m.ReverseProxy,
		FileServer:   m.FileServer,
		Respond:      m.Respond,
		Redir:        m.Redir,
		Rewrite:      m.Rewrite,
		Header:       m.Header,
		Encode:       m.Encode,
		Abort:        m.Abort,
		RawJSON:      m.RawJSON,
	}
}

func (m *handleResourceModel) fromHandle(h handleModel) {
	m.Match = h.Match
	m.ReverseProxy = h.ReverseProxy
	m.FileServer = h.FileServer
	m.Respond = h.Respond
	m.Redir = h.Redir
	m.Rewrite = h.Rewrite
	m.Header = h.Header
	m.Encode = h.Encode
	m.Abort = h.Abort
	m.RawJSON = h.RawJSON
}

func (r *handleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_handle"
}

func (r *handleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Caddy `@id`, equal to `name`.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "Stable `@id` for this handle. Must not use the `{site}__h*` prefix reserved for nested site handles.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"site_id": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "`name`/`id` of the parent `caddy_site`. Changing this forces replacement.",
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.RequiresReplace(),
			},
		},
		"priority": schema.Int64Attribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Optional 0-based insert index among sibling handles. Omit this so existing order is kept; Caddy has no priority field of its own.",
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
			PlanModifiers: []planmodifier.Int64{
				int64planmodifier.UseStateForUnknown(),
			},
		},
		"abort": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Abort the request.",
		},
		"raw_json": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Escape hatch JSON handler object.",
		},
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: "A standalone handle attached to a `caddy_site` subroute. Use this with `for_each` " +
			"when routes are dynamic. Nested `handle` blocks on `caddy_site` stay under `{site}__h*` ids and are not overwritten.",
		Attributes: attrs,
		Blocks:     handlerBlocks(),
	}
}

func (r *handleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req, resp)
}

func (r *handleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data, config handleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ensureAbsent(ctx, r.client, "caddy_handle", data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Create caddy_handle", err.Error())
		return
	}
	index, err := r.upsert(ctx, data, configInsertIndex(config.Priority))
	if err != nil {
		resp.Diagnostics.AddError("Create caddy_handle", err.Error())
		return
	}
	data.ID = data.Name
	data.Priority = appliedPriority(config.Priority, index)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *handleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data handleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, objs, err := loadServers(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_handle", err.Error())
		return
	}

	handle, siteID, index, err := locateHandle(objs, data.SiteID.ValueString(), data.Name.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	_, _, siteRoute, err := findRouteByID(objs, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_handle", err.Error())
		return
	}
	siblings := len(caddyjson.SubrouteRoutes(siteRoute))
	parsed, err := caddyjson.ParseHandle(handle)
	if err != nil {
		resp.Diagnostics.AddError("Parse caddy_handle", err.Error())
		return
	}
	if parsed.ID == "" {
		parsed.ID = data.Name.ValueString()
	}
	model, d := handleFromJSON(ctx, parsed)
	resp.Diagnostics.Append(d...)
	data.fromHandle(model)
	data.Name = types.StringValue(parsed.ID)
	data.SiteID = types.StringValue(siteID)
	data.Priority = settledPriority(data.Priority, index, siblings)
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *handleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, config handleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	index, err := r.upsert(ctx, data, configInsertIndex(config.Priority))
	if err != nil {
		resp.Diagnostics.AddError("Update caddy_handle", err.Error())
		return
	}
	data.ID = data.Name
	data.Priority = appliedPriority(config.Priority, index)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *handleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data handleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.mutateSite(ctx, data.SiteID.ValueString(), func(routes []map[string]any) []map[string]any {
		return caddyjson.RemoveID(routes, data.Name.ValueString())
	}); err != nil && !client.IsMissing(err) {
		resp.Diagnostics.AddError("Delete caddy_handle", err.Error())
	}
}

func (r *handleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := splitImportID(req.ID)
	if len(parts) != 1 && len(parts) != 3 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Use `{server}/{siteIndex|site@id}/{handleIndex|handle@id}` for a live instance, or `{@id}` if the handle already has an @id.\n"+
				"Examples: `srv0/0/1`, `srv0/apps-wildcard/api`, `tenant-acme`.",
		)
		return
	}

	_, objs, err := loadServers(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Import caddy_handle", err.Error())
		return
	}

	var (
		server, siteID, handleID string
		siteIndex, handleIndex   int
		handle                   map[string]any
		route                    map[string]any
	)

	switch len(parts) {
	case 1:
		_, siteID, handleIndex, err = locateHandle(objs, "", parts[0])
		if err != nil {
			resp.Diagnostics.AddError("Import caddy_handle", err.Error())
			return
		}
		handleID = parts[0]
		server, siteIndex, route, err = findRouteByID(objs, siteID)
		if err != nil {
			resp.Diagnostics.AddError("Import caddy_handle", err.Error())
			return
		}
		_ = route
	case 3:
		server = parts[0]
		route, siteIndex, err = findServerRoute(objs, server, parts[1])
		if err != nil {
			resp.Diagnostics.AddError("Import caddy_handle", err.Error())
			return
		}
		siteID, _ = route["@id"].(string)
		if siteID == "" {
			siteID = caddyjson.UniqueID(caddyjson.DefaultSiteName(hostsOf(route), siteIndex), usedIDs(objs))
			if err := stampSiteIdentity(ctx, r.client, server, siteIndex, siteID); err != nil {
				resp.Diagnostics.AddError("Import caddy_handle: assign site @id", err.Error())
				return
			}
			_, objs, _ = loadServers(ctx, r.client)
			route, _, _ = findServerRoute(objs, server, siteID)
		}
		handle, handleIndex, err = findHandle(route, parts[2])
		if err != nil {
			resp.Diagnostics.AddError("Import caddy_handle", err.Error())
			return
		}
		handleID, _ = handle["@id"].(string)
	}

	if handleID == "" {
		handleID = fmt.Sprintf("%s-h%d", siteID, handleIndex)
		if caddyjson.IsNestedHandleID(siteID, handleID) {
			handleID = fmt.Sprintf("%s-handle-%d", siteID, handleIndex)
		}
		if err := stampHandleIdentity(ctx, r.client, server, siteIndex, handleIndex, handleID); err != nil {
			resp.Diagnostics.AddError("Import caddy_handle: assign @id", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), handleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), handleID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site_id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("priority"), int64(handleIndex))...)
}

func locateHandle(objs map[string]map[string]any, siteID, handleRef string) (handle map[string]any, parent string, index int, err error) {
	for _, obj := range objs {
		for _, route := range objectListAny(obj["routes"]) {
			rid, _ := route["@id"].(string)
			if siteID != "" && rid != siteID {
				continue
			}
			h, i, findErr := findHandle(route, handleRef)
			if findErr != nil {
				continue
			}
			if rid == "" {
				continue
			}
			return h, rid, i, nil
		}
	}
	if siteID != "" {
		return nil, "", -1, fmt.Errorf("site %q has no handle %q", siteID, handleRef)
	}
	return nil, "", -1, fmt.Errorf("no handle %q found", handleRef)
}

func (r *handleResource) upsert(ctx context.Context, data handleResourceModel, insertAt int) (int, error) {
	if caddyjson.IsNestedHandleID(data.SiteID.ValueString(), data.Name.ValueString()) {
		return 0, fmt.Errorf("handle name %q uses the reserved nested-handle prefix", data.Name.ValueString())
	}
	h, diags := handleModelToJSON(ctx, data.asHandle())
	if diags.HasError() {
		return 0, diagnosticsError(diags)
	}
	h.ID = data.Name.ValueString()
	compiled, err := caddyjson.CompileHandle(data.SiteID.ValueString(), 0, h, false)
	if err != nil {
		return 0, err
	}
	index := 0
	err = r.mutateSite(ctx, data.SiteID.ValueString(), func(routes []map[string]any) []map[string]any {
		next := caddyjson.InsertRoute(routes, compiled, insertAt)
		index = indexOfID(next, h.ID)
		return next
	})
	return index, err
}

func (r *handleResource) mutateSite(ctx context.Context, siteID string, fn func([]map[string]any) []map[string]any) error {
	return r.client.Locked(func() error {
		raw, etag, err := r.client.GetUnlocked(ctx, "/id/"+siteID)
		if err != nil {
			return err
		}
		site, err := caddyjson.DecodeObject(raw)
		if err != nil {
			return err
		}
		routes := fn(caddyjson.SubrouteRoutes(site))
		caddyjson.SetSubrouteRoutes(site, routes)
		return r.client.PatchUnlocked(ctx, "/id/"+siteID, site, etag)
	})
}
