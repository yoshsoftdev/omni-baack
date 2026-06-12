package meta

import "testing"

func TestParseCoexWebhookPayload_ContactSync(t *testing.T) {
	payload := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{
			"changes": [{
				"field": "smb_app_state_sync",
				"value": {
					"messaging_product": "whatsapp",
					"metadata": { "phone_number_id": "123" },
					"state_sync": [{
						"type": "contact",
						"action": "add",
						"contact": {
							"full_name": "Aulia",
							"phone_number": "628123456789"
						}
					}]
				}
			}]
		}]
	}`)

	events, err := ParseCoexWebhookPayload(payload)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(events.ContactSyncEvents) != 1 {
		t.Fatalf("expected 1 contact sync event, got %d", len(events.ContactSyncEvents))
	}
	if events.ContactSyncEvents[0].FullName != "Aulia" {
		t.Fatalf("unexpected contact name: %s", events.ContactSyncEvents[0].FullName)
	}
}

func TestParseCoexWebhookPayload_HistoryDeclined(t *testing.T) {
	payload := []byte(`{
		"object": "whatsapp_business_account",
		"entry": [{
			"changes": [{
				"field": "history",
				"value": {
					"metadata": { "phone_number_id": "123" },
					"history": [{
						"errors": [{
							"code": 2593109,
							"message": "History sync is turned off"
						}]
					}]
				}
			}]
		}]
	}`)

	events, err := ParseCoexWebhookPayload(payload)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(events.HistoryEvents) != 1 || !events.HistoryEvents[0].Declined {
		t.Fatalf("expected declined history event")
	}
}
