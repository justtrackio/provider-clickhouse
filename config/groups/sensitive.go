package groups

import (
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// SanitizeSensitiveContainers clears the Sensitive flag on any schema field
// that is a *container* (an object, or a list/set/map of objects) rather than a
// scalar.
//
// Why this is needed: Upjet turns a sensitive field into a Kubernetes secret
// reference, and it can only do that for a limited set of types. Its type
// builder fails hard otherwise, e.g.
//
//	cannot infer type from schema of field source: ... got type
//	"*...v1alpha1.CredentialsParameters" for field "Credentials", only types
//	"string", "*string", []string, []*string, "map[string]string" and
//	"map[string]*string" supported as sensitive
//
// The ClickHouse provider marks whole credential *objects* as sensitive, for
// example clickhouse_clickpipe's source.postgres.credentials, in addition to
// marking the secret-bearing leaves inside them (password, username, ...).
// Clearing the flag on the container therefore leaks nothing: every leaf that
// actually holds a secret keeps its own Sensitive flag and is still generated
// as a secret reference and kept out of status.atProvider. The container itself
// only needs to be an ordinary nested struct so that Upjet can build a Go type
// for it.
//
// This is written as a generic recursive walk rather than a hardcoded list of
// field paths so that a Terraform provider version bump which marks a new
// credential object sensitive does not break code generation.
func SanitizeSensitiveContainers() ujconfig.ResourceOption {
	return func(r *ujconfig.Resource) {
		if r.TerraformResource == nil {
			return
		}
		sanitizeSchemaMap(r.TerraformResource.Schema, map[*schema.Resource]bool{})
	}
}

// sanitizeSchemaMap walks a Terraform schema map, clearing Sensitive on every
// non-scalar field. The visited set guards against schemas that are
// self-referential, which would otherwise recurse forever.
func sanitizeSchemaMap(m map[string]*schema.Schema, visited map[*schema.Resource]bool) {
	for _, s := range m {
		if s == nil {
			continue
		}
		elem, isContainer := s.Elem.(*schema.Resource)

		// A field whose element type is a whole resource cannot be
		// expressed as a secret reference.
		if isContainer && s.Sensitive {
			s.Sensitive = false
		}

		// Also clear it for the aggregate types Upjet cannot represent,
		// regardless of element type: only string-valued scalars, lists
		// of strings and maps of strings are supported.
		if s.Sensitive && !isSensitiveSupported(s) {
			s.Sensitive = false
		}

		if isContainer && !visited[elem] {
			visited[elem] = true
			sanitizeSchemaMap(elem.Schema, visited)
		}
	}
}

// isSensitiveSupported reports whether Upjet can represent this schema as a
// secret reference. Mirrors the constraint in Upjet's type builder: a plain
// string, or a list/set/map whose elements are strings.
func isSensitiveSupported(s *schema.Schema) bool {
	switch s.Type {
	case schema.TypeString:
		return true
	case schema.TypeList, schema.TypeSet, schema.TypeMap:
		if s.Elem == nil {
			// A map with no declared element type defaults to string.
			return s.Type == schema.TypeMap
		}
		es, ok := s.Elem.(*schema.Schema)
		return ok && es.Type == schema.TypeString
	case schema.TypeInvalid, schema.TypeBool, schema.TypeInt, schema.TypeFloat:
		// None of these can be expressed as a secret reference.
		return false
	default:
		return false
	}
}
