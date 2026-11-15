package contact

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseCSVUsesExplicitMapping(t *testing.T) {
	csv := "Nama Lengkap,No WA,Kota,Abaikan\nBudi,+628111,Bandung,x\n"
	rows, err := ParseCSV([]byte(csv), map[string]string{"Nama Lengkap": "name", "No WA": "phone", "Kota": "city", "Abaikan": ""})
	if err != nil {
		t.Fatal(err)
	}
	want := ImportRow{Line: 2, Name: "Budi", Phone: "+628111", City: "Bandung"}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("rows = %+v, want [%+v]", rows, want)
	}
}

func TestParseCSVHeaderAliasesAndOrder(t *testing.T) {
	// Known header names map themselves, in any column order.
	rows, err := ParseCSV([]byte("email,phone,name\na@b.c,+628222,Sari\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "Sari" || rows[0].Phone != "+628222" || rows[0].Email != "a@b.c" {
		t.Fatalf("row = %+v", rows[0])
	}
}

func TestParseCSVPositionalWithoutHeader(t *testing.T) {
	rows, err := ParseCSV([]byte("Budi,+628111,b@x.id,vip\n\n,,,\nSari,+628222\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Notes != "vip" || rows[1].Line != 4 || rows[1].Email != "" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestParseCSVRejectsUnmappedPhoneAndOversizedFiles(t *testing.T) {
	if _, err := ParseCSV([]byte("a,b\n1,2\n"), map[string]string{"a": "name"}); err == nil || !strings.Contains(err.Error(), "phone") {
		t.Fatalf("want phone mapping error, got %v", err)
	}
	var b strings.Builder
	b.WriteString("name,phone\n")
	for i := 0; i <= MaxImportRows; i++ {
		fmt.Fprintf(&b, "n,+62%d\n", i)
	}
	if _, err := ParseCSV([]byte(b.String()), nil); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("want row limit error, got %v", err)
	}
}
