package campaign

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/template"
	"github.com/omnichannel/omni-chan-v2/internal/template/builder"
)

// Send validation error codes.
const (
	ValidationCodeMissingVariable        = "missing_variable"
	ValidationCodeInvalidMediaURL        = "invalid_media_url"
	ValidationCodeInvalidMediaReference  = "invalid_media_reference"
	ValidationCodeInvalidButtonParameter = "invalid_button_parameter"
	ValidationCodeValidationError        = "validation_error"
)

// SendValidationError is a permanent pre-send validation failure.
type SendValidationError struct {
	Code    string
	Message string
}

func (e *SendValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// IsSendValidationError reports permanent validation failures that must not hit Meta.
func IsSendValidationError(err error) bool {
	var target *SendValidationError
	return errors.As(err, &target)
}

// ValidateTemplateSend checks required variables and media/button parameters before Meta send.
// variables must be the frozen campaign_recipients.variables snapshot — Contact fallback is forbidden.
func ValidateTemplateSend(
	tpl *template.Template,
	mapping VariableMapping,
	variables map[string]string,
) error {
	if tpl == nil {
		return &SendValidationError{Code: ValidationCodeValidationError, Message: "template is required"}
	}

	mapping = ApplyTemplateHeaderMediaDefaults(mapping, tpl)
	mapping, variables = ApplyTemplateButtonDefaults(tpl, mapping, variables)

	required, err := builder.RequiredFrom(tpl)
	if err != nil {
		return &SendValidationError{Code: ValidationCodeValidationError, Message: err.Error()}
	}

	materialized, err := MaterializeMappingFromSnapshot(mapping, variables)
	if err != nil {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: err.Error(),
		}
	}
	vars := materialized.ToBuilderVariables()
	storageBackedHeader := TemplateHasStorageBackedHeaderMedia(tpl)

	for index := range required.HeaderText {
		if err := validateMappedValue("header", index, vars.HeaderText); err != nil {
			return err
		}
	}
	if required.HeaderMedia {
		ref := vars.HeaderMedia
		hasLink := strings.TrimSpace(ref.Link) != ""
		hasMediaID := strings.TrimSpace(ref.MediaID) != ""
		if !hasLink && !hasMediaID {
			if !storageBackedHeader {
				return &SendValidationError{
					Code:    ValidationCodeMissingVariable,
					Message: "header media reference is required (public image URL or Cloud API media id)",
				}
			}
		} else {
			if hasMediaID && !IsWhatsAppCloudMediaID(ref.MediaID) {
				return &SendValidationError{
					Code:    ValidationCodeInvalidMediaReference,
					Message: "header media_id must be a WhatsApp Cloud API media id (digits only); template header_handle cannot be used for send",
				}
			}
			if err := ref.Validate(); err != nil {
				return &SendValidationError{
					Code:    ValidationCodeInvalidMediaReference,
					Message: err.Error(),
				}
			}
			if ref.UsesLink() {
				if err := validateAbsoluteURL(ref.Link); err != nil {
					return &SendValidationError{
						Code:    ValidationCodeInvalidMediaURL,
						Message: err.Error(),
					}
				}
			}
		}
	}
	for index := range required.Body {
		if err := validateMappedValue("body", index, vars.Body); err != nil {
			return err
		}
	}
	for buttonIndex, urlVars := range required.ButtonURL {
		btnMapping, ok := vars.Buttons[buttonIndex]
		if !ok {
			return &SendValidationError{
				Code:    ValidationCodeInvalidButtonParameter,
				Message: fmt.Sprintf("button %s URL mapping is required", buttonIndex),
			}
		}
		for index := range urlVars {
			if err := validateMappedValue("button:"+buttonIndex, index, btnMapping.URLVars); err != nil {
				return &SendValidationError{
					Code:    ValidationCodeInvalidButtonParameter,
					Message: err.Error(),
				}
			}
		}
	}
	for buttonIndex := range required.ButtonCopy {
		btnMapping, ok := vars.Buttons[buttonIndex]
		if !ok || strings.TrimSpace(btnMapping.CopyCode) == "" {
			return &SendValidationError{
				Code:    ValidationCodeInvalidButtonParameter,
				Message: fmt.Sprintf("button %s copy_code is required", buttonIndex),
			}
		}
	}

	dryVars := vars
	if required.HeaderMedia &&
		strings.TrimSpace(dryVars.HeaderMedia.Link) == "" &&
		strings.TrimSpace(dryVars.HeaderMedia.MediaID) == "" &&
		storageBackedHeader {
		dryVars.HeaderMedia = builder.MediaRef{MediaID: "0"}
	}
	if _, err := builder.Build(tpl, dryVars); err != nil {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: err.Error(),
		}
	}
	return nil
}

func validateMappedValue(scope, index string, mapping map[string]string) error {
	source, ok := mapping[index]
	if !ok || strings.TrimSpace(source) == "" {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: fmt.Sprintf("%s: missing mapping for variable {{%s}}", scope, index),
		}
	}
	value := builder.LiteralResolve(index, source)
	if value == "" {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: fmt.Sprintf("%s: unresolved value for variable {{%s}}", scope, index),
		}
	}
	return nil
}

func validateAbsoluteURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid media URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("media URL must be absolute http(s): %s", raw)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("media URL must include host: %s", raw)
	}
	return nil
}

// RenderValidatedTemplate builds validated send components from a recipient variable snapshot.
func RenderValidatedTemplate(
	tpl *template.Template,
	mapping VariableMapping,
	variables map[string]string,
) ([]builder.SendComponent, error) {
	mapping = ApplyTemplateHeaderMediaDefaults(mapping, tpl)
	mapping, variables = ApplyTemplateButtonDefaults(tpl, mapping, variables)
	if err := ValidateTemplateSend(tpl, mapping, variables); err != nil {
		return nil, err
	}
	materialized, err := MaterializeMappingFromSnapshot(mapping, variables)
	if err != nil {
		return nil, &SendValidationError{Code: ValidationCodeMissingVariable, Message: err.Error()}
	}
	return BuildTemplateComponentsForTemplate(tpl, materialized)
}
