// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"math"

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
	_ resource.Resource                = &tlsPolicyResource{}
	_ resource.ResourceWithConfigure   = &tlsPolicyResource{}
	_ resource.ResourceWithImportState = &tlsPolicyResource{}
)

func NewTLSPolicyResource() resource.Resource { return &tlsPolicyResource{} }

type tlsPolicyResource struct {
	client *client.Client
}

type tlsPolicyModel struct {
	ID       types.String  `tfsdk:"id"`
	Name     types.String  `tfsdk:"name"`
	Subjects types.List    `tfsdk:"subjects"`
	OnDemand types.Bool    `tfsdk:"on_demand"`
	Ask      types.String  `tfsdk:"ask"`
	Issuers  []issuerModel `tfsdk:"issuer"`
}

func (r *tlsPolicyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tls_policy"
}

func (r *tlsPolicyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Caddy TLS automation policy (`apps.tls.automation.policies`). " +
			"Use this for global on-demand TLS (`ask`) and default issuers. Site-level `tls` blocks " +
			"create per-site policies automatically.",
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
				MarkdownDescription: "Policy `@id`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subjects": schema.ListAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Hostnames this policy applies to. Empty means the default policy.",
			},
			"on_demand": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "Enable on-demand TLS for these subjects.",
			},
			"ask": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "On-demand permission endpoint. Caddy GETs `?domain=` and expects HTTP 200 to allow issuance. Required if on_demand is used without another ask source.",
			},
		},
		Blocks: map[string]schema.Block{
			"issuer": issuerBlock(),
		},
	}
}

func (r *tlsPolicyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFrom(req, resp)
}

func (r *tlsPolicyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data tlsPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := ensureAbsent(ctx, r.client, "caddy_tls_policy", data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Create caddy_tls_policy", err.Error())
		return
	}
	if err := r.apply(ctx, data); err != nil {
		resp.Diagnostics.AddError("Create caddy_tls_policy", err.Error())
		return
	}
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *tlsPolicyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data tlsPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := loadPolicies(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Read caddy_tls_policy", err.Error())
		return
	}
	obj, _, err := findPolicy(list, data.Name.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	pol := caddyjson.ParsePolicy(obj)
	if pol.Name == "" {
		pol.Name = data.Name.ValueString()
	}
	subjects, d := listValue(ctx, pol.Subjects)
	resp.Diagnostics.Append(d...)
	data.Subjects = subjects
	data.OnDemand = types.BoolValue(pol.OnDemand)
	data.Name = types.StringValue(pol.Name)
	data.ID = data.Name

	issuers, d := issuersFromJSON(ctx, pol.Issuers, false)
	resp.Diagnostics.Append(d...)
	data.Issuers = issuers

	// ask lives in global on-demand config, so only track it when this
	// policy set it; otherwise another policy's ask would show as drift.
	if !data.Ask.IsNull() {
		data.Ask = stringOrNull(r.readAsk(ctx))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *tlsPolicyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, prior tlsPolicyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, data); err != nil {
		resp.Diagnostics.AddError("Update caddy_tls_policy", err.Error())
		return
	}
	if old := prior.Ask.ValueString(); old != "" && data.Ask.ValueString() == "" {
		if err := r.removeAsk(ctx, old); err != nil {
			resp.Diagnostics.AddError("Update caddy_tls_policy: remove ask", err.Error())
			return
		}
	}
	data.ID = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *tlsPolicyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data tlsPolicyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := removeArrayID(ctx, r.client, tlsPoliciesPath(), data.Name.ValueString()); err != nil && !client.IsMissing(err) {
		resp.Diagnostics.AddError("Delete caddy_tls_policy", err.Error())
		return
	}
	if ask := data.Ask.ValueString(); ask != "" {
		if err := r.removeAsk(ctx, ask); err != nil {
			resp.Diagnostics.AddError("Delete caddy_tls_policy: remove ask", err.Error())
		}
	}
}

// removeAsk deletes the global on-demand config, but only while it still
// points at ask, so a value set by someone else is left alone.
func (r *tlsPolicyResource) removeAsk(ctx context.Context, ask string) error {
	if r.readAsk(ctx) != ask {
		return nil
	}
	err := r.client.Delete(ctx, onDemandPath)
	if client.IsMissing(err) {
		return nil
	}
	return err
}

func (r *tlsPolicyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Use the policy @id or 0-based index in apps.tls.automation.policies. Examples: `tls-apps-wildcard`, `0`.",
		)
		return
	}
	list, err := loadPolicies(ctx, r.client)
	if err != nil {
		resp.Diagnostics.AddError("Import caddy_tls_policy", err.Error())
		return
	}
	obj, index, err := findPolicy(list, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Import caddy_tls_policy", err.Error())
		return
	}
	id, _ := obj["@id"].(string)
	if id == "" {
		id = caddyjson.UniqueID(caddyjson.DefaultSiteName(caddyjson.ParsePolicy(obj).Subjects, index), policyIDs(list))
		if err := stampPolicyIdentity(ctx, r.client, index, id); err != nil {
			resp.Diagnostics.AddError("Import caddy_tls_policy: assign @id", err.Error())
			return
		}
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), id)...)
}

func (r *tlsPolicyResource) readAsk(ctx context.Context) string {
	raw, _, err := r.client.Get(ctx, onDemandPath)
	if err != nil {
		return ""
	}
	obj, err := caddyjson.DecodeObject(raw)
	if err != nil || obj == nil {
		return ""
	}
	ask, _ := obj["ask"].(string)
	return ask
}

const onDemandPath = "/config/apps/tls/automation/on_demand"

func findPolicy(list []map[string]any, ref string) (map[string]any, int, error) {
	if idx, ok := caddyjson.ParseIndex(ref); ok {
		if idx >= len(list) {
			return nil, -1, fmt.Errorf("no tls policy at index %d", idx)
		}
		return list[idx], idx, nil
	}
	for i, p := range list {
		if id, _ := p["@id"].(string); id == ref {
			return p, i, nil
		}
	}
	return nil, -1, fmt.Errorf("no tls policy %q", ref)
}

func policyIDs(list []map[string]any) map[string]struct{} {
	used := map[string]struct{}{}
	for _, p := range list {
		if id, _ := p["@id"].(string); id != "" {
			used[id] = struct{}{}
		}
	}
	return used
}

func (r *tlsPolicyResource) apply(ctx context.Context, data tlsPolicyModel) error {
	if err := r.client.EnsureTLSApp(ctx); err != nil {
		return err
	}
	if err := ensureAutomation(ctx, r.client); err != nil {
		return err
	}
	subjects, diags := listStrings(ctx, data.Subjects)
	if diags.HasError() {
		return diagnosticsError(diags)
	}
	issuers := make([]caddyjson.Issuer, 0, len(data.Issuers))
	for _, iss := range data.Issuers {
		dns, d := dnsToJSON(ctx, iss.DNS)
		if d.HasError() {
			return diagnosticsError(d)
		}
		issuers = append(issuers, caddyjson.Issuer{
			Module: iss.Module.ValueString(),
			CA:     iss.CA.ValueString(),
			Email:  iss.Email.ValueString(),
			DNS:    dns,
		})
	}
	policy, err := caddyjson.CompileTLSPolicy(caddyjson.Policy{
		Name:     data.Name.ValueString(),
		Subjects: subjects,
		OnDemand: data.OnDemand.ValueBool(),
		Issuers:  issuers,
		Ask:      data.Ask.ValueString(),
	})
	if err != nil {
		return err
	}
	// Caddy uses the first policy that matches, so a catch-all policy (no
	// subjects) goes last or it would shadow every site-specific policy.
	insertAt := 0
	if len(subjects) == 0 {
		insertAt = math.MaxInt32
	}
	if err := upsertArray(ctx, r.client, tlsPoliciesPath(), data.Name.ValueString(), policy, insertAt); err != nil {
		return err
	}
	if ask := data.Ask.ValueString(); ask != "" {
		// POST sets or replaces; PUT would fail once on_demand exists.
		return r.client.Post(ctx, onDemandPath, caddyjson.OnDemandAskConfig(ask))
	}
	return nil
}
