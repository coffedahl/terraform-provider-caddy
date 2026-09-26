// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
	"github.com/coffedahl/terraform-provider-caddy/internal/client"
)

var (
	_ resource.Resource                = &serverResource{}
	_ resource.ResourceWithConfigure   = &serverResource{}
	_ resource.ResourceWithImportState = &serverResource{}
)

func NewServerResource() resource.Resource { return &serverResource{} }

type serverResource struct {
	client *client.Client
}

type serverModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Listen           types.List   `tfsdk:"listen"`
	Protocols        types.List   `tfsdk:"protocols"`
	DisableAutoHTTPS types.Bool   `tfsdk:"disable_auto_https"`
	DisableRedirects types.Bool   `tfsdk:"disable_redirects"`
	DisableCerts     types.Bool   `tfsdk:"disable_certificates"`
}

func (r *serverResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_server"
}

func (r *serverResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Caddy HTTP server (`apps.http.servers`). Sites and routes attach to this listener.",
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
				MarkdownDescription: "Server key under `apps.http.servers` and `@id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"listen": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Listener addresses, e.g. `:443` or `:80`.",
			},
			"protocols": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Allowed HTTP protocols (`h1`, `h2`, `h2c`, `h3`).",
			},
			"disable_auto_https": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Disable automatic HTTPS entirely on this server.",
			},
			"disable_redirects": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Keep automatic certs but skip HTTP→HTTPS redirects.",
			},
			"disable_certificates": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Skip certificate automation only.",
			},
		},
	}
}

func (r *serverResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req, resp)
}

func (r *serverResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data serverModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, diags := r.compile(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.EnsureHTTPApp(ctx); err != nil {
		resp.Diagnostics.AddError("Ensure HTTP app", err.Error())
		return
	}
	path := "/config/apps/http/servers/" + url.PathEscape(data.Name.ValueString())
	if err := r.client.Put(ctx, path, obj); err != nil {
		resp.Diagnostics.AddError("Create caddy_server", err.Error())
		return
	}
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data serverModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	raw, _, err := r.client.Get(ctx, "/config/apps/http/servers/"+url.PathEscape(data.Name.ValueString()))
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_server", err.Error())
		return
	}
	obj, err := caddyjson.DecodeObject(raw)
	if err != nil {
		resp.Diagnostics.AddError("Decode caddy_server", err.Error())
		return
	}
	listen, d := listValue(ctx, caddyjsonStringSlice(obj["listen"]))
	resp.Diagnostics.Append(d...)
	data.Listen = listen
	protocols, d := listValue(ctx, caddyjsonStringSlice(obj["protocols"]))
	resp.Diagnostics.Append(d...)
	data.Protocols = protocols
	auto, _ := obj["automatic_https"].(map[string]any)
	data.DisableAutoHTTPS = types.BoolValue(boolFrom(auto, "disable"))
	data.DisableRedirects = types.BoolValue(boolFrom(auto, "disable_redirects"))
	data.DisableCerts = types.BoolValue(boolFrom(auto, "disable_certificates"))
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data serverModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	obj, diags := r.compile(ctx, data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Preserve existing routes when updating listen/TLS flags.
	path := "/config/apps/http/servers/" + url.PathEscape(data.Name.ValueString())
	raw, _, err := r.client.Get(ctx, path)
	if err != nil {
		resp.Diagnostics.AddError("Read existing server", err.Error())
		return
	}
	existing, err := caddyjson.DecodeObject(raw)
	if err == nil {
		if routes, ok := existing["routes"]; ok {
			obj["routes"] = routes
		}
	}
	if err := r.client.Patch(ctx, path, obj); err != nil {
		resp.Diagnostics.AddError("Update caddy_server", err.Error())
		return
	}
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *serverResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serverModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Delete(ctx, "/config/apps/http/servers/"+url.PathEscape(data.Name.ValueString()))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Delete caddy_server", err.Error())
	}
}

func (r *serverResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	name := req.ID
	if name == "" {
		resp.Diagnostics.AddError("Invalid import ID", "Use the HTTP server key from Caddy JSON, e.g. `srv0` or `https`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name)...)
}

func (r *serverResource) compile(ctx context.Context, data serverModel) (map[string]any, diag.Diagnostics) {
	listen, diags := listStrings(ctx, data.Listen)
	protocols, d := listStrings(ctx, data.Protocols)
	diags.Append(d...)
	obj, err := caddyjson.CompileServer(caddyjson.Server{
		Name:             data.Name.ValueString(),
		Listen:           listen,
		Protocols:        protocols,
		DisableAutoHTTPS: data.DisableAutoHTTPS.ValueBool(),
		DisableRedirects: data.DisableRedirects.ValueBool(),
		DisableCerts:     data.DisableCerts.ValueBool(),
	})
	if err != nil {
		diags.AddError("Compile caddy_server", err.Error())
	}
	return obj, diags
}

func caddyjsonStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func boolFrom(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	b, _ := m[key].(bool)
	return b
}
