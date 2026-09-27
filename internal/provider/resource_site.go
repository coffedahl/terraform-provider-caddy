// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                = &siteResource{}
	_ resource.ResourceWithConfigure   = &siteResource{}
	_ resource.ResourceWithImportState = &siteResource{}
)

func NewSiteResource() resource.Resource { return &siteResource{} }

type siteResource struct {
	client *client.Client
}

type siteModel struct {
	ID       types.String  `tfsdk:"id"`
	Name     types.String  `tfsdk:"name"`
	ServerID types.String  `tfsdk:"server_id"`
	Hosts    types.List    `tfsdk:"hosts"`
	Terminal types.Bool    `tfsdk:"terminal"`
	Priority types.Int64   `tfsdk:"priority"`
	TLS      []tlsModel    `tfsdk:"tls"`
	Handles  []handleModel `tfsdk:"handle"`
}

func (r *siteResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *siteResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	blocks := handleNestedBlocks(true)
	blocks["tls"] = tlsBlock()

	resp.Schema = schema.Schema{
		MarkdownDescription: "A Caddy site: a top-level host matcher wrapping nested handles in a subroute. " +
			"Use `hosts = [\"*.example.com\"]` for a wildcard certificate covering one DNS label, then put " +
			"per-path or per-subdomain routes in `handle` blocks. Automatic HTTPS is used unless `tls` is set.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Caddy `@id`, equal to `name`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Stable `@id` for this site route.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"server_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name/`id` of the `caddy_server` this site is attached to. Changing this forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hosts": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Hostnames. `*.example.com` matches `foo.example.com` but not `foo.bar.example.com`.",
			},
			"terminal": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Stop evaluating later top-level routes on match. Caddyfile site blocks are terminal.",
			},
			"priority": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Optional 0-based insert index among the server's routes. Omit it to keep the current order.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: blocks,
	}
}

func (r *siteResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req, resp)
}

func (r *siteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data, config siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ensureAbsent(ctx, r.client, "caddy_site", data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Create caddy_site", err.Error())
		return
	}
	index, err := r.apply(ctx, data, configInsertIndex(config.Priority))
	if err != nil {
		resp.Diagnostics.AddError("Create caddy_site", err.Error())
		return
	}
	data.ID = data.Name
	data.Priority = appliedPriority(config.Priority, index)
	fillHandlePriorities(data.Handles)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *siteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, objs, err := loadServers(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_site", err.Error())
		return
	}

	var (
		route map[string]any
		index int
	)
	server := data.ServerID.ValueString()
	name := data.Name.ValueString()

	if server != "" {
		route, index, err = findServerRoute(objs, server, name)
		if err != nil {
			// Fall back to scanning by @id in case the import id was just the name.
			var scanErr error
			server, index, route, scanErr = findRouteByID(objs, name)
			if scanErr != nil {
				resp.State.RemoveResource(ctx)
				return
			}
		}
	} else {
		server, index, route, err = findRouteByID(objs, name)
		if err != nil {
			resp.State.RemoveResource(ctx)
			return
		}
	}

	parsed, err := caddyjson.ParseSite(route)
	if err != nil {
		resp.Diagnostics.AddError("Parse caddy_site", err.Error())
		return
	}
	if parsed.Name == "" {
		parsed.Name = name
	}
	hosts, d := listValue(ctx, parsed.Hosts)
	resp.Diagnostics.Append(d...)
	data.Hosts = hosts
	data.Terminal = types.BoolValue(parsed.Terminal)
	data.Priority = settledPriority(data.Priority, index, len(objectListAny(objs[server]["routes"])))
	data.ServerID = types.StringValue(server)
	data.Name = types.StringValue(parsed.Name)
	data.ID = data.Name

	nested := make([]handleModel, 0)
	for _, h := range parsed.Handles {
		if h.ID != "" && !caddyjson.IsNestedHandleID(data.Name.ValueString(), h.ID) {
			continue
		}
		model, d := handleFromJSON(ctx, h)
		resp.Diagnostics.Append(d...)
		model.Name = types.StringNull()
		// Nested handles keep declaration order, so priority is the position.
		model.Priority = types.Int64Value(int64(len(nested)))
		nested = append(nested, model)
	}
	data.Handles = nested

	tls, d := r.readSiteTLS(ctx, data.Name.ValueString(), parsed.Hosts, len(data.TLS) > 0)
	resp.Diagnostics.Append(d...)
	data.TLS = tls

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *siteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, config siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	index, err := r.apply(ctx, data, configInsertIndex(config.Priority))
	if err != nil {
		resp.Diagnostics.AddError("Update caddy_site", err.Error())
		return
	}
	data.ID = data.Name
	data.Priority = appliedPriority(config.Priority, index)
	fillHandlePriorities(data.Handles)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *siteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := removeArrayID(ctx, r.client, serverRoutesPath(data.ServerID.ValueString()), data.Name.ValueString()); err != nil && !client.IsMissing(err) {
		resp.Diagnostics.AddError("Delete caddy_site", err.Error())
		return
	}
	if err := r.client.DeleteID(ctx, caddyjson.TLSPolicyID(data.Name.ValueString())); err != nil && !client.IsMissing(err) {
		resp.Diagnostics.AddError("Delete caddy_site: remove TLS policy", err.Error())
		return
	}
	if err := removeLoadFile(ctx, r.client, data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Delete caddy_site: remove certificate load_files entry", err.Error())
	}
}

func (r *siteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := splitImportID(req.ID)
	if len(parts) == 0 || len(parts) > 2 {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Use `{server}/{index|@id}` for a live Caddyfile-style route, or `{@id}` if the route already has an @id.\n"+
				"Examples: `srv0/0`, `srv0/apps-wildcard`, `apps-wildcard`.",
		)
		return
	}

	_, objs, err := loadServers(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Import caddy_site", err.Error())
		return
	}

	var (
		server string
		index  int
		route  map[string]any
	)
	switch len(parts) {
	case 1:
		server, index, route, err = findRouteByID(objs, parts[0])
	case 2:
		server = parts[0]
		route, index, err = findServerRoute(objs, server, parts[1])
	}
	if err != nil {
		resp.Diagnostics.AddError("Import caddy_site", err.Error())
		return
	}

	siteID, _ := route["@id"].(string)
	if siteID == "" {
		siteID = caddyjson.UniqueID(caddyjson.DefaultSiteName(hostsOf(route), index), usedIDs(objs))
		if err := stampSiteIdentity(ctx, r.client, server, index, siteID); err != nil {
			resp.Diagnostics.AddError("Import caddy_site: assign @id", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), siteID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("server_id"), server)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("priority"), int64(index))...)
}

// readSiteTLS finds the site's automation policy. Matching by subjects is
// only a fallback for sites that already have a tls block in state (such as
// imported Caddyfile sites), so a separate caddy_tls_policy covering the same
// hosts is never claimed by the site.
func (r *siteResource) readSiteTLS(ctx context.Context, siteName string, hosts []string, hadTLS bool) ([]tlsModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	list, err := loadPolicies(ctx, r.client)
	if err != nil {
		diags.AddError("Read caddy_site TLS policy", err.Error())
		return nil, diags
	}
	want := caddyjson.TLSPolicyID(siteName)
	var match map[string]any
	for _, p := range list {
		if id, _ := p["@id"].(string); id == want {
			match = p
			break
		}
	}
	if match == nil && hadTLS {
		for _, p := range list {
			if sameStrings(caddyjson.ParsePolicy(p).Subjects, hosts) {
				match = p
				break
			}
		}
	}
	if match == nil {
		return nil, nil
	}
	t := policyToTLS(caddyjson.ParsePolicy(match))
	cert, key, err := r.readLoadFile(ctx, siteName)
	if err != nil {
		diags.AddError("Read caddy_site certificate", err.Error())
		return nil, diags
	}
	t.CertificateFile, t.KeyFile = cert, key
	return tlsFromJSON(ctx, t)
}

func (r *siteResource) readLoadFile(ctx context.Context, siteName string) (string, string, error) {
	raw, _, err := r.client.Get(ctx, loadFilesPath)
	if client.IsMissing(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	list, err := caddyjson.DecodeObjectList(raw)
	if err != nil {
		return "", "", err
	}
	for _, entry := range list {
		if firstTag(entry) == siteName {
			cert, _ := entry["certificate"].(string)
			key, _ := entry["key"].(string)
			return cert, key, nil
		}
	}
	return "", "", nil
}

func sameStrings(a, b []string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}

func (r *siteResource) apply(ctx context.Context, data siteModel, insertAt int) (int, error) {
	site, diags := r.toJSON(ctx, data)
	if diags.HasError() {
		return 0, diagnosticsError(diags)
	}
	compiled, err := caddyjson.CompileSite(site)
	if err != nil {
		return 0, err
	}

	if err := r.client.EnsureHTTPApp(ctx); err != nil {
		return 0, err
	}

	index := 0
	err = r.client.Locked(func() error {
		path := serverRoutesPath(data.ServerID.ValueString())
		raw, etag, err := r.client.GetUnlocked(ctx, path)
		var routes []map[string]any
		if err != nil {
			if !client.IsNotFound(err) {
				return err
			}
		} else {
			routes, err = caddyjson.DecodeObjectList(raw)
			if err != nil {
				return err
			}
			for _, existing := range routes {
				if id, _ := existing["@id"].(string); id == site.Name {
					merged := caddyjson.MergeSiteHandles(
						caddyjson.SubrouteRoutes(existing),
						caddyjson.SubrouteRoutes(compiled),
						site.Name,
					)
					caddyjson.SetSubrouteRoutes(compiled, merged)
					break
				}
			}
		}
		routes = caddyjson.InsertRoute(routes, compiled, insertAt)
		index = indexOfID(routes, site.Name)
		if patchErr := r.client.PatchUnlocked(ctx, path, routes, etag); patchErr != nil {
			if client.IsNotFound(patchErr) {
				return r.client.PutUnlocked(ctx, path, routes, "")
			}
			return patchErr
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if err := r.applyTLS(ctx, site); err != nil {
		return index, err
	}
	return index, nil
}

func indexOfID(routes []map[string]any, id string) int {
	for i, r := range routes {
		if rid, _ := r["@id"].(string); rid == id {
			return i
		}
	}
	if len(routes) == 0 {
		return 0
	}
	return len(routes) - 1
}

func fillHandlePriorities(handles []handleModel) {
	for i := range handles {
		if handles[i].Priority.IsUnknown() || handles[i].Priority.IsNull() {
			handles[i].Priority = types.Int64Value(int64(i))
		}
	}
}

func (r *siteResource) applyTLS(ctx context.Context, site caddyjson.Site) error {
	if site.TLS == nil {
		if err := r.client.DeleteID(ctx, caddyjson.TLSPolicyID(site.Name)); err != nil && !client.IsMissing(err) {
			return err
		}
		return removeLoadFile(ctx, r.client, site.Name)
	}
	if err := r.client.EnsureTLSApp(ctx); err != nil {
		return err
	}
	policy, err := caddyjson.CompileSiteTLSPolicy(site)
	if err != nil {
		return err
	}
	if policy != nil {
		if err := ensureAutomation(ctx, r.client); err != nil {
			return err
		}
		if err := upsertArray(ctx, r.client, tlsPoliciesPath(), caddyjson.TLSPolicyID(site.Name), policy, 0); err != nil {
			return err
		}
	}
	if load := caddyjson.CompileLoadFiles(site); load != nil {
		if err := upsertLoadFile(ctx, r.client, load); err != nil {
			return err
		}
	}
	return nil
}

func (r *siteResource) toJSON(ctx context.Context, data siteModel) (caddyjson.Site, diag.Diagnostics) {
	hosts, diags := listStrings(ctx, data.Hosts)
	handles, d := handleToJSON(ctx, data.Handles)
	diags.Append(d...)
	tls, d := tlsToJSON(ctx, data.TLS)
	diags.Append(d...)
	return caddyjson.Site{
		Name:     data.Name.ValueString(),
		Hosts:    hosts,
		Terminal: data.Terminal.ValueBool(),
		Priority: int(data.Priority.ValueInt64()),
		TLS:      tls,
		Handles:  handles,
	}, diags
}

func ensureAutomation(ctx context.Context, c *client.Client) error {
	return c.Locked(func() error {
		raw, _, err := c.GetUnlocked(ctx, "/config/apps/tls")
		if err != nil && !client.IsNotFound(err) {
			return err
		}
		obj := map[string]any{}
		if err == nil {
			obj, _ = caddyjson.DecodeObject(raw)
			if obj == nil {
				obj = map[string]any{}
			}
		}
		automation, _ := obj["automation"].(map[string]any)
		if automation == nil {
			return c.PutUnlocked(ctx, "/config/apps/tls/automation", map[string]any{
				"policies": []any{},
			}, "")
		}
		if _, ok := automation["policies"]; !ok {
			return c.PutUnlocked(ctx, tlsPoliciesPath(), []any{}, "")
		}
		return nil
	})
}

func diagnosticsError(diags diag.Diagnostics) error {
	for _, d := range diags {
		if d.Severity() == diag.SeverityError {
			return fmt.Errorf("%s: %s", d.Summary(), d.Detail())
		}
	}
	return nil
}
