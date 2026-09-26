// Copyright (c) 2026 terraform-provider-caddy contributors
// SPDX-License-Identifier: MPL-2.0

package caddyjson

// Site is the HCL-level model for a Caddy site (top-level host route).
type Site struct {
	Name     string
	Hosts    []string
	Terminal bool
	Priority int
	TLS      *TLS
	Handles  []Handle
}

// Handle is one subroute entry: matchers plus exactly one primary handler.
type Handle struct {
	ID           string
	Priority     int
	Match        *Match
	ReverseProxy *ReverseProxy
	FileServer   *FileServer
	Respond      *Respond
	Redir        *Redir
	Rewrite      *Rewrite
	Header       *Header
	Encode       *Encode
	Abort        bool
	RawJSON      string
}

// Match is a Caddy matcher set (AND of fields; multiple sets are OR).
type Match struct {
	Host   []string
	Path   []string
	Method []string
	Header map[string][]string
}

// ReverseProxy is http.handlers.reverse_proxy.
type ReverseProxy struct {
	Upstreams             []Upstream
	HeaderUp              map[string][]string
	HeaderDown            map[string][]string
	TransportTLS          bool
	TLSInsecureSkipVerify bool
	TLSServerName         string
	TLSClientCertFile     string
	TLSClientKeyFile      string
	Selection             string
	HealthURI             string
	HealthInterval        string
}

// Upstream is a reverse_proxy upstream.
type Upstream struct {
	Dial string
}

// FileServer is http.handlers.file_server.
type FileServer struct {
	Root          string
	Browse        bool
	Hide          []string
	IndexNames    []string
	Precompressed []string
}

// Respond is http.handlers.static_response.
type Respond struct {
	StatusCode int
	Body       string
	Close      bool
	Headers    map[string][]string
}

// Redir compiles to static_response with a Location header.
type Redir struct {
	To         string
	StatusCode int
}

// Rewrite is http.handlers.rewrite.
type Rewrite struct {
	URI             string
	StripPathPrefix string
	StripPathSuffix string
	URIPrefix       string
	URISuffix       string
	TryFiles        []string
}

// Header is http.handlers.headers.
type Header struct {
	ResponseSet    map[string][]string
	ResponseDelete []string
	RequestSet     map[string][]string
	RequestDelete  []string
}

// Encode is http.handlers.encode.
type Encode struct {
	Encodings []string
}

// TLS is site-level certificate automation.
type TLS struct {
	Internal        bool
	OnDemand        bool
	CertificateFile string
	KeyFile         string
	Issuers         []Issuer
}

// Issuer is a tls.issuance module, typically acme.
type Issuer struct {
	Module string
	CA     string
	Email  string
	DNS    *DNSProvider
}

// DNSProvider is a caddy-dns module used for the ACME DNS-01 challenge.
type DNSProvider struct {
	Name               string
	Config             map[string]string
	Resolvers          []string
	PropagationDelay   string
	PropagationTimeout string
	TTL                string
	OverrideDomain     string
}

// Policy is apps.tls.automation.policies.
type Policy struct {
	Name     string
	Subjects []string
	OnDemand bool
	Issuers  []Issuer
	Ask      string
}

// Server is apps.http.servers.{name}.
type Server struct {
	Name             string
	Listen           []string
	Protocols        []string
	DisableAutoHTTPS bool
	DisableRedirects bool
	DisableCerts     bool
}
