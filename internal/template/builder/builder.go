package builder

import (
	"fmt"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// SendComponent is the Meta Cloud API send-time component shape.
// Defined here so Campaign/Inbox depend on Template Builder, not pkg/meta.
type SendComponent = metapkg.TemplateComponent

// MediaRef identifies header media by public URL or Cloud API media id.
type MediaRef struct {
	Link    string
	MediaID string
}

// Validate ensures exactly one of link or media_id is set.
func (r MediaRef) Validate() error {
	link := strings.TrimSpace(r.Link)
	id := strings.TrimSpace(r.MediaID)
	switch {
	case link == "" && id == "":
		return fmt.Errorf("header media reference is required")
	case link != "" && id != "":
		return fmt.Errorf("header media must use either link or media_id, not both")
	}
	return nil
}

// UsesLink reports whether the reference targets a public URL.
func (r MediaRef) UsesLink() bool {
	return strings.TrimSpace(r.Link) != ""
}

// UsesMediaID reports whether the reference targets a Cloud API media id.
func (r MediaRef) UsesMediaID() bool {
	return strings.TrimSpace(r.MediaID) != ""
}

// ButtonVars maps button-level variables for send-time rendering.
type ButtonVars struct {
	URLVars  map[string]string
	CopyCode string
}

// Variables carries resolved send-time values for a Template.
// Callers (Campaign) must materialize Contact/snapshot sources before Build.
type Variables struct {
	HeaderText  map[string]string
	HeaderMedia MediaRef
	Body        map[string]string
	Buttons     map[string]ButtonVars
}

// Required describes send-time values required by a Template definition.
type Required struct {
	HeaderText  map[string]struct{}
	HeaderMedia bool
	Body        map[string]struct{}
	ButtonURL   map[string]map[string]struct{}
	ButtonCopy  map[string]struct{}
}

// ResolveFunc resolves a mapping source for a variable index.
// Builder does not know Contact — sources must already be literals or snapshot values.
type ResolveFunc func(index, source string) string

// LiteralResolve returns the mapping source trimmed (snapshot/literal path).
func LiteralResolve(_, source string) string {
	return strings.TrimSpace(source)
}

// RequiredFrom inspects a domain Template for required send variables.
func RequiredFrom(tmpl *template.Template) (Required, error) {
	if tmpl == nil {
		return Required{}, fmt.Errorf("template is required")
	}
	metaReq, err := metapkg.RequiredTemplateVariables(tmpl.Components)
	if err != nil {
		return Required{}, err
	}
	return Required{
		HeaderText:  metaReq.HeaderText,
		HeaderMedia: metaReq.HeaderMedia,
		Body:        metaReq.Body,
		ButtonURL:   metaReq.ButtonURL,
		ButtonCopy:  metaReq.ButtonCopy,
	}, nil
}

// Build renders Meta send components from a domain Template and Variables.
// Meta Graph DTO conversion happens only at this package edge.
func Build(tmpl *template.Template, vars Variables) ([]SendComponent, error) {
	return BuildWithResolve(tmpl, vars, LiteralResolve)
}

// BuildWithResolve is Build with a custom resolver (tests / advanced callers).
func BuildWithResolve(tmpl *template.Template, vars Variables, resolve ResolveFunc) ([]SendComponent, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("template is required")
	}
	if resolve == nil {
		resolve = LiteralResolve
	}
	payload, err := metapkg.BuildTemplatePayload(tmpl.Components, toMetaMapping(vars), metapkg.ResolveFunc(resolve))
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, nil
	}
	return payload.Components, nil
}

func toMetaMapping(vars Variables) metapkg.VariableMappingInput {
	out := metapkg.VariableMappingInput{
		HeaderText: vars.HeaderText,
		HeaderMedia: metapkg.MediaReference{
			Link:    vars.HeaderMedia.Link,
			MediaID: vars.HeaderMedia.MediaID,
		},
		Body:    vars.Body,
		Buttons: make(map[string]metapkg.ButtonMappingInput, len(vars.Buttons)),
	}
	for index, btn := range vars.Buttons {
		out.Buttons[index] = metapkg.ButtonMappingInput{
			URLVars:  btn.URLVars,
			CopyCode: btn.CopyCode,
		}
	}
	return out
}
