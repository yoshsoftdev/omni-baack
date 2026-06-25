package meta

import (
	"fmt"
	"strings"
)

// MediaReference identifies header media by public URL or Meta media ID.
type MediaReference struct {
	Link    string
	MediaID string
}

// Validate ensures exactly one of link or media_id is set.
func (r MediaReference) Validate() error {
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

// ToMediaObject returns the Meta payload media object.
func (r MediaReference) ToMediaObject() (MediaObject, error) {
	if err := r.Validate(); err != nil {
		return MediaObject{}, err
	}
	if id := strings.TrimSpace(r.MediaID); id != "" {
		return MediaObject{ID: id}, nil
	}
	return MediaObject{Link: strings.TrimSpace(r.Link)}, nil
}

// UsesMediaID reports whether the reference targets a Meta upload handle.
func (r MediaReference) UsesMediaID() bool {
	return strings.TrimSpace(r.MediaID) != ""
}

// UsesLink reports whether the reference targets a public URL.
func (r MediaReference) UsesLink() bool {
	return strings.TrimSpace(r.Link) != ""
}
