package meta

import "testing"

func TestSummarizeWebhookPayload(t *testing.T) {
	body := []byte(`{
		"object":"whatsapp_business_account",
		"entry":[{"changes":[{"field":"messages","value":{
			"metadata":{"phone_number_id":"123"},
			"messages":[{"from":"6281","id":"wamid.1","type":"text","text":{"body":"hello"}}]
		}}]}]
	}`)

	summary, err := SummarizeWebhookPayload(body)
	if err != nil {
		t.Fatalf("SummarizeWebhookPayload() error = %v", err)
	}
	if summary.TextMessageCount != 1 {
		t.Fatalf("TextMessageCount = %d, want 1", summary.TextMessageCount)
	}
	if len(summary.PhoneNumberIDs) != 1 || summary.PhoneNumberIDs[0] != "123" {
		t.Fatalf("PhoneNumberIDs = %#v", summary.PhoneNumberIDs)
	}
}
