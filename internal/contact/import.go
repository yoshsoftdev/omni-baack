package contact

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

type ImportRow struct {
	Name  string
	Phone string
	Email string
	Notes string
}

func ParseCSV(data []byte) ([]ImportRow, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("csv file is empty")
	}

	start := 0
	if isHeaderRow(records[0]) {
		start = 1
	}

	rows := make([]ImportRow, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 || strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		row := ImportRow{
			Name:  fieldAt(record, 0),
			Phone: fieldAt(record, 1),
			Email: fieldAt(record, 2),
			Notes: fieldAt(record, 3),
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func ParseExcel(data []byte) ([]ImportRow, error) {
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
	if len(records) == 0 {
		return nil, fmt.Errorf("excel file is empty")
	}

	start := 0
	if isHeaderRow(records[0]) {
		start = 1
	}

	rows := make([]ImportRow, 0, len(records)-start)
	for i := start; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 || strings.TrimSpace(strings.Join(record, "")) == "" {
			continue
		}
		rows = append(rows, ImportRow{
			Name:  fieldAt(record, 0),
			Phone: fieldAt(record, 1),
			Email: fieldAt(record, 2),
			Notes: fieldAt(record, 3),
		})
	}
	return rows, nil
}

func isHeaderRow(record []string) bool {
	if len(record) < 2 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(record[0]))
	second := strings.ToLower(strings.TrimSpace(record[1]))
	return first == "name" && (second == "phone" || second == "phone_number" || second == "whatsapp")
}

func fieldAt(record []string, index int) string {
	if index >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[index])
}

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
