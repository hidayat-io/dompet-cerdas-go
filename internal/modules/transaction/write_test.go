package transaction

import (
	"errors"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

func TestResolveManualCategoryID_OverrideWins(t *testing.T) {
	got, err := ResolveManualCategoryID("Makanan", "c9", sampleCategories)
	if err != nil {
		t.Fatalf("ResolveManualCategoryID: %v", err)
	}
	if got != "c9" {
		t.Errorf("id = %s, want the override c9", got)
	}
}

func TestResolveManualCategoryID_ExactNameCaseInsensitive(t *testing.T) {
	got, err := ResolveManualCategoryID("mAkAnAn", "", sampleCategories)
	if err != nil {
		t.Fatalf("ResolveManualCategoryID: %v", err)
	}
	if got != "c1" {
		t.Errorf("id = %s, want c1", got)
	}
}

func TestResolveManualCategoryID_UnknownNameFallsBack(t *testing.T) {
	got, err := ResolveManualCategoryID("Tidak Ada", "", sampleCategories)
	if err != nil {
		t.Fatalf("ResolveManualCategoryID: %v", err)
	}
	if got != "c3" {
		t.Errorf("id = %s, want c3 (Belanja fallback)", got)
	}
}

// An override must work even with no categories loaded — that is what lets the
// batch skip the category read entirely.
func TestResolveManualCategoryID_OverrideWorksWithoutCategories(t *testing.T) {
	got, err := ResolveManualCategoryID("", "c1", nil)
	if err != nil {
		t.Fatalf("ResolveManualCategoryID: %v", err)
	}
	if got != "c1" {
		t.Errorf("id = %s, want c1", got)
	}
}

func TestResolveManualCategoryID_NoCategoriesNoOverrideIsAnError(t *testing.T) {
	if _, err := ResolveManualCategoryID("Makanan", "", nil); !errors.Is(err, ErrNoCategories) {
		t.Errorf("err = %v, want ErrNoCategories", err)
	}
}

func TestBuildManualPayload(t *testing.T) {
	payload := BuildManualPayload(25000, "c1", "Makan siang", "u1", "Budi", nil)

	if payload["amount"] != int64(25000) {
		t.Errorf("amount = %v, want 25000", payload["amount"])
	}
	if payload["categoryId"] != "c1" {
		t.Errorf("categoryId = %v, want c1", payload["categoryId"])
	}
	if payload["source"] != string(domain.TransactionSourceTelegram) {
		t.Errorf("source = %v, want telegram", payload["source"])
	}
	if payload["date"] != datetime.TodayString() {
		t.Errorf("date = %v, want today in Jakarta (%s)", payload["date"], datetime.TodayString())
	}
	if payload["createdByUserId"] != "u1" || payload["createdByName"] != "Budi" {
		t.Errorf("creator fields = %v/%v, want u1/Budi", payload["createdByUserId"], payload["createdByName"])
	}
}

// Receipt photos are stored privately, so the attachment carries a Storage path
// but no URL — the web app resolves one from the path (ADR-017).
func TestBuildManualPayload_WritesAttachment(t *testing.T) {
	attachment := &domain.Attachment{
		Path: "users/u1/accounts/a1/attachments/receipt_123.jpg",
		Type: domain.AttachmentTypeImage,
		Name: "receipt_123.jpg",
		Size: 4096,
	}

	got, ok := BuildManualPayload(25000, "c1", "Makan siang", "u1", "Budi", attachment)["attachment"].(map[string]interface{})
	if !ok {
		t.Fatal("attachment must be written as a map")
	}
	if got["path"] != attachment.Path {
		t.Errorf("path = %v, want %s", got["path"], attachment.Path)
	}
	if got["url"] != "" {
		t.Errorf("url = %v, want empty — no public or signed URL is persisted", got["url"])
	}
	if got["type"] != "image" || got["name"] != attachment.Name || got["size"] != int64(4096) {
		t.Errorf("metadata = %v", got)
	}
}

func TestBuildManualPayload_OmitsAttachmentWhenNil(t *testing.T) {
	if _, present := BuildManualPayload(1, "c1", "x", "u1", "Budi", nil)["attachment"]; present {
		t.Error("attachment must be absent, not empty, when there is none")
	}
}

// The legacy document has no "id" field; adding one would make Telegram rows
// differ from the ones the web app writes.
func TestBuildManualPayload_HasNoIDField(t *testing.T) {
	if _, present := BuildManualPayload(1, "c1", "x", "u1", "Budi", nil)["id"]; present {
		t.Error("payload must not contain an id field")
	}
}

// Creator fields are omitted rather than written empty, matching
// sanitizeFirestoreData dropping undefined values.
func TestBuildManualPayload_OmitsCreatorWhenUnknown(t *testing.T) {
	payload := BuildManualPayload(1, "c1", "x", "", "", nil)

	if _, present := payload["createdByUserId"]; present {
		t.Error("createdByUserId must be absent, not empty")
	}
	if _, present := payload["createdByName"]; present {
		t.Error("createdByName must be absent, not empty")
	}
}
