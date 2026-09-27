// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type handleModel struct {
	Name         types.String        `tfsdk:"name"`
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

type matchModel struct {
	Host   types.List `tfsdk:"host"`
	Path   types.List `tfsdk:"path"`
	Method types.List `tfsdk:"method"`
	Header types.Map  `tfsdk:"header"`
}

type reverseProxyModel struct {
	Upstreams             []upstreamModel `tfsdk:"upstream"`
	HeaderUp              types.Map       `tfsdk:"header_up"`
	HeaderDown            types.Map       `tfsdk:"header_down"`
	TransportTLS          types.Bool      `tfsdk:"transport_tls"`
	TLSInsecureSkipVerify types.Bool      `tfsdk:"tls_insecure_skip_verify"`
	TLSServerName         types.String    `tfsdk:"tls_server_name"`
	TLSClientCertFile     types.String    `tfsdk:"tls_client_certificate_file"`
	TLSClientKeyFile      types.String    `tfsdk:"tls_client_certificate_key_file"`
	Selection             types.String    `tfsdk:"selection_policy"`
	HealthURI             types.String    `tfsdk:"health_uri"`
	HealthInterval        types.String    `tfsdk:"health_interval"`
}

type upstreamModel struct {
	Dial types.String `tfsdk:"dial"`
}

type fileServerModel struct {
	Root          types.String `tfsdk:"root"`
	Browse        types.Bool   `tfsdk:"browse"`
	Hide          types.List   `tfsdk:"hide"`
	IndexNames    types.List   `tfsdk:"index_names"`
	Precompressed types.List   `tfsdk:"precompressed"`
}

type respondModel struct {
	StatusCode types.Int64  `tfsdk:"status_code"`
	Body       types.String `tfsdk:"body"`
	Close      types.Bool   `tfsdk:"close"`
	Headers    types.Map    `tfsdk:"headers"`
}

type redirModel struct {
	To         types.String `tfsdk:"to"`
	StatusCode types.Int64  `tfsdk:"status_code"`
}

type rewriteModel struct {
	URI             types.String `tfsdk:"uri"`
	StripPathPrefix types.String `tfsdk:"strip_path_prefix"`
	StripPathSuffix types.String `tfsdk:"strip_path_suffix"`
	URIPrefix       types.String `tfsdk:"uri_prefix"`
	URISuffix       types.String `tfsdk:"uri_suffix"`
	TryFiles        types.List   `tfsdk:"try_files"`
}

type headerModel struct {
	ResponseSet    types.Map  `tfsdk:"response_set"`
	ResponseDelete types.List `tfsdk:"response_delete"`
	RequestSet     types.Map  `tfsdk:"request_set"`
	RequestDelete  types.List `tfsdk:"request_delete"`
}

type encodeModel struct {
	Encodings types.List `tfsdk:"encodings"`
}

type tlsModel struct {
	Internal        types.Bool    `tfsdk:"internal"`
	OnDemand        types.Bool    `tfsdk:"on_demand"`
	CertificateFile types.String  `tfsdk:"certificate_file"`
	KeyFile         types.String  `tfsdk:"key_file"`
	Issuers         []issuerModel `tfsdk:"issuer"`
}

type issuerModel struct {
	Module types.String `tfsdk:"module"`
	CA     types.String `tfsdk:"ca"`
	Email  types.String `tfsdk:"email"`
	DNS    []dnsModel   `tfsdk:"dns"`
}

type dnsModel struct {
	Name               types.String `tfsdk:"name"`
	Config             types.Map    `tfsdk:"config"`
	Resolvers          types.List   `tfsdk:"resolvers"`
	PropagationDelay   types.String `tfsdk:"propagation_delay"`
	PropagationTimeout types.String `tfsdk:"propagation_timeout"`
	TTL                types.String `tfsdk:"ttl"`
	OverrideDomain     types.String `tfsdk:"override_domain"`
}

func handleNestedBlocks(includeName bool) map[string]schema.Block {
	attrs := map[string]schema.Attribute{
		"priority": schema.Int64Attribute{
			Optional:            true,
			Computed:            true,
			MarkdownDescription: "Optional 0-based insert index. Omit it to keep the current order (or append when creating). Do not set every handle to 0.",
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"abort": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Abort the request (Caddy `abort` handler).",
		},
		"raw_json": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Escape hatch: a JSON object for any Caddy HTTP handler module. Must include `handler`.",
		},
	}
	if includeName {
		attrs["name"] = schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Stable `@id` for this handle. Nested site handles get `{site}__h{index}` if omitted.",
		}
	}

	return map[string]schema.Block{
		"handle": schema.ListNestedBlock{
			MarkdownDescription: "Ordered handlers inside this site. Declaration order is preserved. " +
				"Set exactly one of reverse_proxy, file_server, respond, redir, rewrite, header, encode, abort, or raw_json.",
			NestedObject: schema.NestedBlockObject{
				Attributes: attrs,
				Blocks:     handlerBlocks(),
			},
		},
	}
}

func atMostOne() []validator.List {
	return []validator.List{listvalidator.SizeAtMost(1)}
}

func handlerBlocks() map[string]schema.Block {
	return map[string]schema.Block{
		"match": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "AND matcher set. Multiple fields must all match.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"host": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Host matcher. `*.example.com` matches one label.",
					},
					"path": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Path matcher. `/api/*` is a prefix match.",
					},
					"method": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "HTTP methods.",
					},
					"header": schema.MapAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Header name to required value.",
					},
				},
			},
		},
		"reverse_proxy": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `reverse_proxy` handler.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"header_up": schema.MapAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Request headers to set on the upstream request.",
					},
					"header_down": schema.MapAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Response headers to set on the downstream response.",
					},
					"transport_tls": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
						MarkdownDescription: "Dial the upstream with TLS (`transport http { tls }`). Implied by the other tls_* attributes.",
					},
					"tls_insecure_skip_verify": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
						MarkdownDescription: "Skip verification of the upstream TLS certificate (Caddyfile `tls_insecure_skip_verify`). Needed for Proxmox and other internal HTTPS with a default cert.",
					},
					"tls_server_name": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "SNI server name when dialing the upstream over TLS.",
					},
					"tls_client_certificate_file": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Client certificate PEM file for upstream mTLS.",
					},
					"tls_client_certificate_key_file": schema.StringAttribute{
						Optional:            true,
						Sensitive:           true,
						MarkdownDescription: "Client certificate key PEM file for upstream mTLS.",
					},
					"selection_policy": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Load balancing policy (`round_robin`, `first`, `least_conn`, ...).",
					},
					"health_uri": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Active health check URI.",
					},
					"health_interval": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Active health check interval (Go duration, e.g. `10s`).",
					},
				},
				Blocks: map[string]schema.Block{
					"upstream": schema.ListNestedBlock{
						MarkdownDescription: "Backends. Each `dial` is a single address (`host:port` or unix socket).",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"dial": schema.StringAttribute{
									Required:            true,
									MarkdownDescription: "Dial address, e.g. `127.0.0.1:8080` or `unix//run/app.sock`.",
								},
							},
						},
					},
				},
			},
		},
		"file_server": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `file_server` handler.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"root": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Site root directory.",
					},
					"browse": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
						MarkdownDescription: "Enable directory browsing.",
					},
					"hide": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Paths to hide.",
					},
					"index_names": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Index filenames.",
					},
					"precompressed": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Precompressed encodings to look for (`br`, `gzip`, `zstd`).",
					},
				},
			},
		},
		"respond": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `static_response` / `respond`.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"status_code": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "HTTP status code.",
					},
					"body": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Response body.",
					},
					"close": schema.BoolAttribute{
						Optional:            true,
						Computed:            true,
						Default:             booldefault.StaticBool(false),
						MarkdownDescription: "Close the client connection after writing.",
					},
					"headers": schema.MapAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "Response headers.",
					},
				},
			},
		},
		"redir": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "HTTP redirect (compiled to `static_response` + Location).",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"to": schema.StringAttribute{
						Required:            true,
						MarkdownDescription: "Location value. May include Caddy placeholders.",
					},
					"status_code": schema.Int64Attribute{
						Optional:            true,
						MarkdownDescription: "Redirect status. Defaults to 302.",
					},
				},
			},
		},
		"rewrite": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `rewrite` handler. `try_files` emits a file matcher plus rewrite.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"uri": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Replace the URI.",
					},
					"strip_path_prefix": schema.StringAttribute{Optional: true},
					"strip_path_suffix": schema.StringAttribute{Optional: true},
					"uri_prefix":        schema.StringAttribute{Optional: true},
					"uri_suffix":        schema.StringAttribute{Optional: true},
					"try_files": schema.ListAttribute{
						Optional:            true,
						ElementType:         types.StringType,
						MarkdownDescription: "SPA-style try_files list, e.g. `[{http.request.uri.path}, /index.html]`.",
					},
				},
			},
		},
		"header": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `header` handler.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"response_set": schema.MapAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
					"response_delete": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
					"request_set": schema.MapAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
					"request_delete": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
		},
		"encode": schema.ListNestedBlock{
			Validators:          atMostOne(),
			MarkdownDescription: "Caddy `encode` handler. Defaults to gzip and zstd if encodings is omitted.",
			NestedObject: schema.NestedBlockObject{
				Attributes: map[string]schema.Attribute{
					"encodings": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
		},
	}
}

func issuerBlock() schema.ListNestedBlock {
	return schema.ListNestedBlock{
		MarkdownDescription: "Certificate issuers (ACME, internal, zerossl, ...).",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"module": schema.StringAttribute{
					Optional:            true,
					Computed:            true,
					Default:             stringdefault.StaticString("acme"),
					MarkdownDescription: "Issuer module. Defaults to `acme`.",
				},
				"ca": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "ACME directory URL. Use Let's Encrypt staging while testing.",
				},
				"email": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "ACME account email.",
				},
			},
			Blocks: map[string]schema.Block{
				"dns": schema.ListNestedBlock{
					Validators:          atMostOne(),
					MarkdownDescription: "ACME DNS-01 provider (`caddy-dns` module). Required for wildcard certificates.",
					NestedObject: schema.NestedBlockObject{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required:            true,
								MarkdownDescription: "Module name, e.g. `cloudflare` or `route53`.",
							},
							"config": schema.MapAttribute{
								Optional:            true,
								Sensitive:           true,
								ElementType:         types.StringType,
								MarkdownDescription: "Provider-specific fields (tokens, keys). Marked sensitive.",
							},
							"resolvers": schema.ListAttribute{
								Optional:            true,
								ElementType:         types.StringType,
								MarkdownDescription: "DNS resolvers used during the DNS-01 challenge, e.g. `[\"1.1.1.1\", \"1.0.0.1\"]`.",
							},
							"propagation_delay": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "How long to wait before checking TXT propagation (Go duration, e.g. `5m`).",
							},
							"propagation_timeout": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "How long to wait for TXT records to propagate (Go duration, e.g. `20m`).",
							},
							"ttl": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "TTL of the ACME DNS TXT record (Go duration).",
							},
							"override_domain": schema.StringAttribute{
								Optional:            true,
								MarkdownDescription: "Delegate the DNS-01 challenge to this domain (`dns_challenge_override_domain`).",
							},
						},
					},
				},
			},
		},
	}
}

func tlsBlock() schema.Block {
	return schema.ListNestedBlock{
		Validators: atMostOne(),
		MarkdownDescription: "Optional TLS automation for this site's hosts. Omit to use Caddy automatic HTTPS. " +
			"Wildcard names require `issuer.dns` and a Caddy build that includes that DNS module.",
		NestedObject: schema.NestedBlockObject{
			Attributes: map[string]schema.Attribute{
				"internal": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(false),
					MarkdownDescription: "Use Caddy's internal CA (`tls.issuance.internal`).",
				},
				"on_demand": schema.BoolAttribute{
					Optional:            true,
					Computed:            true,
					Default:             booldefault.StaticBool(false),
					MarkdownDescription: "Obtain certificates at handshake time. Requires a global `caddy_tls_policy` ask endpoint.",
				},
				"certificate_file": schema.StringAttribute{
					Optional:            true,
					MarkdownDescription: "Path to a PEM certificate to load instead of automating.",
				},
				"key_file": schema.StringAttribute{
					Optional:            true,
					Sensitive:           true,
					MarkdownDescription: "Path to the certificate private key.",
				},
			},
			Blocks: map[string]schema.Block{
				"issuer": issuerBlock(),
			},
		},
	}
}
