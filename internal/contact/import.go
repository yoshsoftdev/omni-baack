package contact

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
	"github.com/xuri/excelize/v2"
)

// MaxImportRows caps one synchronous import.
// shortcut: imports run inside the request; move to internal/importjob (async + progress polling) when files outgrow this.
const MaxImportRows = 5000

// MaxImportBytes caps the uploaded file size.
const MaxImportBytes = 10 << 20

// ImportRow is one data row of an import file. Line is its 1-based line in the file.
type ImportRow struct {
	Line    int
	Name    string
	Phone   string
	Email   string
	Notes   string
	City    string
	Company string
}

// importFields are the contact fields a file column can be mapped to.
var importFields = map[string]bool{"name": true, "phone": true, "email": true, "notes": true, "city": true, "company": true}

// headerAliases map common header names to fields when no explicit mapping is given for a column.
var headerAliases = map[string]string{
	"name": "name", "contact_name": "name", "customer_name": "name", "nama": "name",
	"phone": "phone", "phone_number": "phone", "whatsapp": "phone", "telepon": "phone", "no_hp": "phone",
	"email": "email", "notes": "notes", "city": "city", "kota": "city", "company": "company",
}

// ParseCSV reads import rows. mapping is "file column header" → field ("" skips the column).
func ParseCSV(data []byte, mapping map[string]string) ([]ImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	var records [][]string
	var lines []int // the reader skips blank lines, so keep each record's real line for error reports
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse csv: %w", err)
		}
		line, _ := reader.FieldPos(0)
		records = append(records, record)
		lines = append(lines, line)
	}
	return rowsFromRecords(records, lines, mapping)
}

// ParseExcel reads import rows from the first sheet; mapping works as in ParseCSV.
func ParseExcel(data []byte, mapping map[string]string) ([]ImportRow, error) {
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse excel: %w", err)
	}
	defer func() { _ = file.Close() }()

	sheet := file.GetSheetName(0)
	if sheet == "" {
		return nil, fmt.Errorf("excel file has no sheets")
	}
	records, err := file.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read excel rows: %w", err)
	}
	lines := make([]int, len(records))
	for i := range records {
		lines[i] = i + 1
	}
	return rowsFromRecords(records, lines, mapping)
}

func rowsFromRecords(records [][]string, lines []int, mapping map[string]string) ([]ImportRow, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	// Without a header the old fixed order applies: name, phone, email, notes.
	columns := map[string]int{"name": 0, "phone": 1, "email": 2, "notes": 3}
	start := 0
	if len(mapping) > 0 || isHeaderRow(records[0]) {
		columns = headerColumns(records[0], mapping)
		start = 1
	}
	if _, ok := columns["phone"]; !ok {
		return nil, fmt.Errorf("no column is mapped to phone")
	}
	if len(records)-start > MaxImportRows {
		return nil, fmt.Errorf("file has %d rows; the limit is %d", len(records)-start, MaxImportRows)
	}

	rows := make([]ImportRow, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		record := records[i]
		if strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		get := func(field string) string {
			index, ok := columns[field]
			if !ok {
				return ""
			}
			return fieldAt(record, index)
		}
		rows = append(rows, ImportRow{
			Line:    lines[i],
			Name:    get("name"),
			Phone:   get("phone"),
			Email:   get("email"),
			Notes:   get("notes"),
			City:    get("city"),
			Company: get("company"),
		})
	}
	return rows, nil
}

func headerColumns(header []string, mapping map[string]string) map[string]int {
	columns := map[string]int{}
	for index, raw := range header {
		key := strings.TrimSpace(raw)
		field, explicit := mapping[key]
		if !explicit {
			field = headerAliases[strings.ToLower(key)]
		}
		if _, taken := columns[field]; importFields[field] && !taken {
			columns[field] = index
		}
	}
	return columns
}

// isHeaderRow reports whether the first row names its columns: a data row never holds a word like "phone".
func isHeaderRow(record []string) bool {
	for _, cell := range record {
		if headerAliases[strings.ToLower(strings.TrimSpace(cell))] == "phone" {
			return true
		}
	}
	return false
}

func fieldAt(record []string, index int) string {
	if index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

// ReadUploadFile reads at most maxBytes, failing when the file is larger.
func ReadUploadFile(reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file too large")
	}
	return data, nil
}

// ImportResult reports an import row by row.
type ImportResult struct {
	Created int              `json:"created"`
	Updated int              `json:"updated"`
	Skipped int              `json:"skipped"`
	Errors  []ImportRowError `json:"errors,omitempty"`
}

// ImportRowError explains why one file line was skipped.
type ImportRowError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func optionalField(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// Import creates contacts for new phone numbers and fills in non-empty fields on existing ones.
// Bad rows are skipped and reported; an infrastructure error aborts the import.
func (s *Service) Import(ctx context.Context, teamID, userID uuid.UUID, actorRole string, rows []ImportRow) (*ImportResult, error) {
	if err := s.authorize(actorRole, rbac.PermContactCreate); err != nil {
		return nil, err
	}
	if err := s.authorize(actorRole, rbac.PermContactUpdate); err != nil {
		return nil, err
	}
	result := &ImportResult{}
	skip := func(line int, message string) {
		result.Skipped++
		result.Errors = append(result.Errors, ImportRowError{Line: line, Message: message})
	}

	for i, row := range rows {
		if row.Phone == "" {
			skip(row.Line, "phone is required")
			continue
		}
		phone, err := normalizePhone(row.Phone)
		if err != nil {
			skip(row.Line, err.Error())
			continue
		}
		existing, err := s.contacts.FindByPhone(ctx, teamID, phone)
		switch {
		case err == nil:
			_, err = s.Update(ctx, teamID, userID, actorRole, existing.ID, UpdateInput{
				ContactName: optionalField(row.Name),
				Email:       optionalField(row.Email),
				Notes:       optionalField(row.Notes),
				City:        optionalField(row.City),
				Company:     optionalField(row.Company),
			})
			if err == nil {
				result.Updated++
			}
		case errors.Is(err, repository.ErrNotFound):
			name := row.Name
			if name == "" {
				name = phone
			}
			_, err = s.Create(ctx, teamID, userID, actorRole, CreateInput{
				Name:    name,
				Phone:   phone,
				Email:   optionalField(row.Email),
				Notes:   optionalField(row.Notes),
				City:    optionalField(row.City),
				Company: optionalField(row.Company),
				Source:  SourceImport,
			})
			if err == nil {
				result.Created++
			}
		default:
			return nil, serviceerrors.MapRepository(err)
		}

		switch {
		case err == nil:
		case errors.Is(err, serviceerrors.ErrQuotaExceeded):
			for _, rest := range rows[i:] {
				skip(rest.Line, "contact quota reached")
			}
			return result, nil
		case serviceerrors.IsValidation(err), errors.Is(err, serviceerrors.ErrConflict):
			skip(row.Line, err.Error())
		default:
			return nil, err
		}
	}
	return result, nil
}
