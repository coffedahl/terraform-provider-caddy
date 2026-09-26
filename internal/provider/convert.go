// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/coffedahl/terraform-provider-caddy/internal/caddyjson"
)

func listStrings(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	if l.IsNull() || l.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := l.ElementsAs(ctx, &out, false)
	return out, diags
}

func listValue(ctx context.Context, values []string) (types.List, diag.Diagnostics) {
	if values == nil {
		return types.ListNull(types.StringType), nil
	}
	return types.ListValueFrom(ctx, types.StringType, values)
}

func mapStringToHeader(ctx context.Context, m types.Map) (map[string][]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	raw := map[string]string{}
	diags := m.ElementsAs(ctx, &raw, false)
	if diags.HasError() {
		return nil, diags
	}
	out := make(map[string][]string, len(raw))
	for k, v := range raw {
		out[k] = []string{v}
	}
	return out, diags
}

func headerToMap(ctx context.Context, headers map[string][]string) (types.Map, diag.Diagnostics) {
	if len(headers) == 0 {
		return types.MapNull(types.StringType), nil
	}
	raw := make(map[string]string, len(headers))
	for k, vs := range headers {
		if len(vs) == 0 {
			raw[k] = ""
			continue
		}
		raw[k] = vs[0]
	}
	return types.MapValueFrom(ctx, types.StringType, raw)
}

func stringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	out := map[string]string{}
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

func handleToJSON(ctx context.Context, models []handleModel) ([]caddyjson.Handle, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]caddyjson.Handle, 0, len(models))
	for _, m := range models {
		h, d := handleModelToJSON(ctx, m)
		diags.Append(d...)
		out = append(out, h)
	}
	return out, diags
}

func handleModelToJSON(ctx context.Context, m handleModel) (caddyjson.Handle, diag.Diagnostics) {
	var diags diag.Diagnostics
	h := caddyjson.Handle{
		ID:      m.Name.ValueString(),
		Abort:   m.Abort.ValueBool(),
		RawJSON: m.RawJSON.ValueString(),
	}
	if !m.Priority.IsNull() && !m.Priority.IsUnknown() {
		h.Priority = int(m.Priority.ValueInt64())
	}
	if len(m.Match) == 1 {
		hosts, d := listStrings(ctx, m.Match[0].Host)
		diags.Append(d...)
		paths, d := listStrings(ctx, m.Match[0].Path)
		diags.Append(d...)
		methods, d := listStrings(ctx, m.Match[0].Method)
		diags.Append(d...)
		header, d := mapStringToHeader(ctx, m.Match[0].Header)
		diags.Append(d...)
		h.Match = &caddyjson.Match{Host: hosts, Path: paths, Method: methods, Header: header}
	}
	if len(m.ReverseProxy) == 1 {
		rp := m.ReverseProxy[0]
		p := &caddyjson.ReverseProxy{
			Selection:             rp.Selection.ValueString(),
			HealthURI:             rp.HealthURI.ValueString(),
			HealthInterval:        rp.HealthInterval.ValueString(),
			TransportTLS:          rp.TransportTLS.ValueBool(),
			TLSInsecureSkipVerify: rp.TLSInsecureSkipVerify.ValueBool(),
			TLSServerName:         rp.TLSServerName.ValueString(),
			TLSClientCertFile:     rp.TLSClientCertFile.ValueString(),
			TLSClientKeyFile:      rp.TLSClientKeyFile.ValueString(),
		}
		p.HeaderUp, _ = mapStringToHeader(ctx, rp.HeaderUp)
		p.HeaderDown, _ = mapStringToHeader(ctx, rp.HeaderDown)
		for _, u := range rp.Upstreams {
			p.Upstreams = append(p.Upstreams, caddyjson.Upstream{Dial: u.Dial.ValueString()})
		}
		h.ReverseProxy = p
	}
	if len(m.FileServer) == 1 {
		fs := m.FileServer[0]
		hide, d := listStrings(ctx, fs.Hide)
		diags.Append(d...)
		index, d := listStrings(ctx, fs.IndexNames)
		diags.Append(d...)
		pre, d := listStrings(ctx, fs.Precompressed)
		diags.Append(d...)
		h.FileServer = &caddyjson.FileServer{
			Root:          fs.Root.ValueString(),
			Browse:        fs.Browse.ValueBool(),
			Hide:          hide,
			IndexNames:    index,
			Precompressed: pre,
		}
	}
	if len(m.Respond) == 1 {
		rs := m.Respond[0]
		headers, d := mapStringToHeader(ctx, rs.Headers)
		diags.Append(d...)
		h.Respond = &caddyjson.Respond{
			StatusCode: int(rs.StatusCode.ValueInt64()),
			Body:       rs.Body.ValueString(),
			Close:      rs.Close.ValueBool(),
			Headers:    headers,
		}
	}
	if len(m.Redir) == 1 {
		h.Redir = &caddyjson.Redir{
			To:         m.Redir[0].To.ValueString(),
			StatusCode: int(m.Redir[0].StatusCode.ValueInt64()),
		}
	}
	if len(m.Rewrite) == 1 {
		rw := m.Rewrite[0]
		try, d := listStrings(ctx, rw.TryFiles)
		diags.Append(d...)
		h.Rewrite = &caddyjson.Rewrite{
			URI:             rw.URI.ValueString(),
			StripPathPrefix: rw.StripPathPrefix.ValueString(),
			StripPathSuffix: rw.StripPathSuffix.ValueString(),
			URIPrefix:       rw.URIPrefix.ValueString(),
			URISuffix:       rw.URISuffix.ValueString(),
			TryFiles:        try,
		}
	}
	if len(m.Header) == 1 {
		hd := m.Header[0]
		rs, d := mapStringToHeader(ctx, hd.ResponseSet)
		diags.Append(d...)
		rq, d := mapStringToHeader(ctx, hd.RequestSet)
		diags.Append(d...)
		rd, d := listStrings(ctx, hd.ResponseDelete)
		diags.Append(d...)
		qd, d := listStrings(ctx, hd.RequestDelete)
		diags.Append(d...)
		h.Header = &caddyjson.Header{
			ResponseSet:    rs,
			RequestSet:     rq,
			ResponseDelete: rd,
			RequestDelete:  qd,
		}
	}
	if len(m.Encode) == 1 {
		enc, d := listStrings(ctx, m.Encode[0].Encodings)
		diags.Append(d...)
		h.Encode = &caddyjson.Encode{Encodings: enc}
	}
	return h, diags
}

func handleFromJSON(ctx context.Context, h caddyjson.Handle) (handleModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	m := handleModel{
		Name:     types.StringNull(),
		Priority: types.Int64Value(int64(h.Priority)),
		Abort:    types.BoolValue(h.Abort),
		RawJSON:  types.StringNull(),
	}
	if h.ID != "" {
		m.Name = types.StringValue(h.ID)
	}
	if h.RawJSON != "" {
		m.RawJSON = types.StringValue(h.RawJSON)
	}
	if h.Match != nil {
		hosts, d := listValue(ctx, h.Match.Host)
		diags.Append(d...)
		paths, d := listValue(ctx, h.Match.Path)
		diags.Append(d...)
		methods, d := listValue(ctx, h.Match.Method)
		diags.Append(d...)
		header, d := headerToMap(ctx, h.Match.Header)
		diags.Append(d...)
		m.Match = []matchModel{{Host: hosts, Path: paths, Method: methods, Header: header}}
	}
	if h.ReverseProxy != nil {
		up := make([]upstreamModel, 0, len(h.ReverseProxy.Upstreams))
		for _, u := range h.ReverseProxy.Upstreams {
			up = append(up, upstreamModel{Dial: types.StringValue(u.Dial)})
		}
		hu, d := headerToMap(ctx, h.ReverseProxy.HeaderUp)
		diags.Append(d...)
		hd, d := headerToMap(ctx, h.ReverseProxy.HeaderDown)
		diags.Append(d...)
		m.ReverseProxy = []reverseProxyModel{{
			Upstreams:             up,
			HeaderUp:              hu,
			HeaderDown:            hd,
			TransportTLS:          types.BoolValue(h.ReverseProxy.TransportTLS),
			TLSInsecureSkipVerify: types.BoolValue(h.ReverseProxy.TLSInsecureSkipVerify),
			TLSServerName:         stringOrNull(h.ReverseProxy.TLSServerName),
			TLSClientCertFile:     stringOrNull(h.ReverseProxy.TLSClientCertFile),
			TLSClientKeyFile:      stringOrNull(h.ReverseProxy.TLSClientKeyFile),
			Selection:             stringOrNull(h.ReverseProxy.Selection),
			HealthURI:             stringOrNull(h.ReverseProxy.HealthURI),
			HealthInterval:        stringOrNull(h.ReverseProxy.HealthInterval),
		}}
	}
	if h.FileServer != nil {
		hide, d := listValue(ctx, h.FileServer.Hide)
		diags.Append(d...)
		index, d := listValue(ctx, h.FileServer.IndexNames)
		diags.Append(d...)
		pre, d := listValue(ctx, h.FileServer.Precompressed)
		diags.Append(d...)
		m.FileServer = []fileServerModel{{
			Root:          stringOrNull(h.FileServer.Root),
			Browse:        types.BoolValue(h.FileServer.Browse),
			Hide:          hide,
			IndexNames:    index,
			Precompressed: pre,
		}}
	}
	if h.Respond != nil {
		headers, d := headerToMap(ctx, h.Respond.Headers)
		diags.Append(d...)
		m.Respond = []respondModel{{
			StatusCode: intOrNull(h.Respond.StatusCode),
			Body:       stringOrNull(h.Respond.Body),
			Close:      types.BoolValue(h.Respond.Close),
			Headers:    headers,
		}}
	}
	if h.Redir != nil {
		m.Redir = []redirModel{{
			To:         types.StringValue(h.Redir.To),
			StatusCode: intOrNull(h.Redir.StatusCode),
		}}
	}
	if h.Rewrite != nil {
		try, d := listValue(ctx, h.Rewrite.TryFiles)
		diags.Append(d...)
		m.Rewrite = []rewriteModel{{
			URI:             stringOrNull(h.Rewrite.URI),
			StripPathPrefix: stringOrNull(h.Rewrite.StripPathPrefix),
			StripPathSuffix: stringOrNull(h.Rewrite.StripPathSuffix),
			URIPrefix:       stringOrNull(h.Rewrite.URIPrefix),
			URISuffix:       stringOrNull(h.Rewrite.URISuffix),
			TryFiles:        try,
		}}
	}
	if h.Header != nil {
		rs, d := headerToMap(ctx, h.Header.ResponseSet)
		diags.Append(d...)
		rq, d := headerToMap(ctx, h.Header.RequestSet)
		diags.Append(d...)
		rd, d := listValue(ctx, h.Header.ResponseDelete)
		diags.Append(d...)
		qd, d := listValue(ctx, h.Header.RequestDelete)
		diags.Append(d...)
		m.Header = []headerModel{{
			ResponseSet:    rs,
			RequestSet:     rq,
			ResponseDelete: rd,
			RequestDelete:  qd,
		}}
	}
	if h.Encode != nil {
		enc, d := listValue(ctx, h.Encode.Encodings)
		diags.Append(d...)
		m.Encode = []encodeModel{{Encodings: enc}}
	}
	return m, diags
}

func configInsertIndex(v types.Int64) int {
	if v.IsNull() || v.IsUnknown() {
		return caddyjson.PriorityUnspecified
	}
	return int(v.ValueInt64())
}

func stringOrNull(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func intOrNull(v int) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(v))
}

func tlsToJSON(ctx context.Context, models []tlsModel) (*caddyjson.TLS, diag.Diagnostics) {
	if len(models) == 0 {
		return nil, nil
	}
	m := models[0]
	var diags diag.Diagnostics
	out := &caddyjson.TLS{
		Internal:        m.Internal.ValueBool(),
		OnDemand:        m.OnDemand.ValueBool(),
		CertificateFile: m.CertificateFile.ValueString(),
		KeyFile:         m.KeyFile.ValueString(),
	}
	for _, iss := range m.Issuers {
		dns, d := dnsToJSON(ctx, iss.DNS)
		diags.Append(d...)
		out.Issuers = append(out.Issuers, caddyjson.Issuer{
			Module: iss.Module.ValueString(),
			CA:     iss.CA.ValueString(),
			Email:  iss.Email.ValueString(),
			DNS:    dns,
		})
	}
	return out, diags
}

func tlsFromJSON(ctx context.Context, t *caddyjson.TLS) ([]tlsModel, diag.Diagnostics) {
	if t == nil {
		return nil, nil
	}
	var diags diag.Diagnostics
	m := tlsModel{
		Internal:        types.BoolValue(t.Internal),
		OnDemand:        types.BoolValue(t.OnDemand),
		CertificateFile: stringOrNull(t.CertificateFile),
		KeyFile:         stringOrNull(t.KeyFile),
	}
	for _, iss := range t.Issuers {
		im := issuerModel{
			Module: stringOrNull(iss.Module),
			CA:     stringOrNull(iss.CA),
			Email:  stringOrNull(iss.Email),
		}
		if iss.DNS != nil {
			cfgMap := iss.DNS.Config
			if cfgMap == nil {
				cfgMap = map[string]string{}
			}
			cfg, d := types.MapValueFrom(ctx, types.StringType, cfgMap)
			diags.Append(d...)
			resolvers, d := listValue(ctx, iss.DNS.Resolvers)
			diags.Append(d...)
			im.DNS = []dnsModel{{
				Name:               types.StringValue(iss.DNS.Name),
				Config:             cfg,
				Resolvers:          resolvers,
				PropagationDelay:   stringOrNull(iss.DNS.PropagationDelay),
				PropagationTimeout: stringOrNull(iss.DNS.PropagationTimeout),
				TTL:                stringOrNull(iss.DNS.TTL),
				OverrideDomain:     stringOrNull(iss.DNS.OverrideDomain),
			}}
		}
		if iss.Module == "internal" {
			m.Internal = types.BoolValue(true)
			continue
		}
		m.Issuers = append(m.Issuers, im)
	}
	return []tlsModel{m}, diags
}

func policyToTLS(p caddyjson.Policy) *caddyjson.TLS {
	t := &caddyjson.TLS{
		OnDemand: p.OnDemand,
		Issuers:  p.Issuers,
	}
	for _, iss := range p.Issuers {
		if iss.Module == "internal" {
			t.Internal = true
		}
	}
	return t
}

func dnsToJSON(ctx context.Context, models []dnsModel) (*caddyjson.DNSProvider, diag.Diagnostics) {
	if len(models) == 0 {
		return nil, nil
	}
	m := models[0]
	cfg, diags := stringMap(ctx, m.Config)
	resolvers, d := listStrings(ctx, m.Resolvers)
	diags.Append(d...)
	return &caddyjson.DNSProvider{
		Name:               m.Name.ValueString(),
		Config:             cfg,
		Resolvers:          resolvers,
		PropagationDelay:   m.PropagationDelay.ValueString(),
		PropagationTimeout: m.PropagationTimeout.ValueString(),
		TTL:                m.TTL.ValueString(),
		OverrideDomain:     m.OverrideDomain.ValueString(),
	}, diags
}
