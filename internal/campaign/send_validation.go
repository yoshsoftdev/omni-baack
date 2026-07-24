package campaign

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/omnichannel/omni-chan-v2/internal/contact"
	"github.com/omnichannel/omni-chan-v2/internal/template"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

// Send validation error codes.
const (
	ValidationCodeMissingVariable       = "missing_variable"
	ValidationCodeInvalidMediaURL        = "invalid_media_url"
	ValidationCodeInvalidMediaReference  = "invalid_media_reference"
	ValidationCodeInvalidButtonParameter = "invalid_button_parameter"
	ValidationCodeValidationError       = "validation_error"
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
func ValidateTemplateSend(
	tpl *template.Template,
	mapping VariableMapping,
	contact *contact.Contact,
	sampleValues map[string]string,
) error {
	if tpl == nil {
		return &SendValidationError{Code: ValidationCodeValidationError, Message: "template is required"}
	}

	mapping = ApplyTemplateHeaderMediaDefaults(mapping, tpl)

	required, err := metaclient.RequiredTemplateVariables(tpl.Components)
	if err != nil {
		return &SendValidationError{Code: ValidationCodeValidationError, Message: err.Error()}
	}

	resolve := NewVariableResolver(contact, sampleValues)
	metaMapping := mapping.ToMetaMapping()
	storageBackedHeader := TemplateHasStorageBackedHeaderMedia(tpl)

	for index := range required.HeaderText {
		if err := validateMappedValue("header", index, metaMapping.HeaderText, resolve); err != nil {
			return err
		}
	}
	if required.HeaderMedia {
		ref := metaMapping.HeaderMedia
		hasLink := strings.TrimSpace(ref.Link) != ""
		hasMediaID := strings.TrimSpace(ref.MediaID) != ""
		if !hasLink && !hasMediaID {
			if !storageBackedHeader {
				return &SendValidationError{
					Code:    ValidationCodeMissingVariable,
					Message: "header media reference is required (public image URL or Cloud API media id)",
				}
			}
			// Worker uploads from storage_object_id at send time; dry-run uses a placeholder below.
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
		if err := validateMappedValue("body", index, metaMapping.Body, resolve); err != nil {
			return err
		}
	}
	for buttonIndex, vars := range required.ButtonURL {
		btnMapping, ok := metaMapping.Buttons[buttonIndex]
		if !ok {
			return &SendValidationError{
				Code:    ValidationCodeInvalidButtonParameter,
				Message: fmt.Sprintf("button %s URL mapping is required", buttonIndex),
			}
		}
		for index := range vars {
			if err := validateMappedValue("button:"+buttonIndex, index, btnMapping.URLVars, resolve); err != nil {
				return &SendValidationError{
					Code:    ValidationCodeInvalidButtonParameter,
					Message: err.Error(),
				}
			}
		}
	}
	for buttonIndex := range required.ButtonCopy {
		btnMapping, ok := metaMapping.Buttons[buttonIndex]
		if !ok || strings.TrimSpace(btnMapping.CopyCode) == "" {
			return &SendValidationError{
				Code:    ValidationCodeInvalidButtonParameter,
				Message: fmt.Sprintf("button %s copy_code is required", buttonIndex),
			}
		}
	}

	// Dry-run render to catch builder-level gaps before Meta API call.
	dryMapping := metaMapping
	if required.HeaderMedia &&
		strings.TrimSpace(dryMapping.HeaderMedia.Link) == "" &&
		strings.TrimSpace(dryMapping.HeaderMedia.MediaID) == "" &&
		storageBackedHeader {
		dryMapping.HeaderMedia = metaclient.MediaReference{MediaID: "0"}
	}
	if _, err := metaclient.BuildTemplatePayload(tpl.Components, dryMapping, resolve); err != nil {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: err.Error(),
		}
	}
	return nil
}

func validateMappedValue(scope, index string, mapping map[string]string, resolve metaclient.ResolveFunc) error {
	source, ok := mapping[index]
	if !ok || strings.TrimSpace(source) == "" {
		return &SendValidationError{
			Code:    ValidationCodeMissingVariable,
			Message: fmt.Sprintf("%s: missing mapping for variable {{%s}}", scope, index),
		}
	}
	value := strings.TrimSpace(resolve(index, source))
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

// RenderValidatedTemplate builds validated Meta components for send.
func RenderValidatedTemplate(
	tpl *template.Template,
	mapping VariableMapping,
	contact *contact.Contact,
	sampleValues map[string]string,
) ([]metaclient.TemplateComponent, error) {
	mapping = ApplyTemplateHeaderMediaDefaults(mapping, tpl)
	if err := ValidateTemplateSend(tpl, mapping, contact, sampleValues); err != nil {
		return nil, err
	}
	return BuildTemplateComponents(tpl.Components, mapping, contact, sampleValues)
}
