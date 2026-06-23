package repository

import "testing"

func TestPageParamsNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   PageParams
		want PageParams
	}{
		{
			name: "defaults",
			in:   PageParams{},
			want: PageParams{Page: DefaultPage, PageSize: DefaultPageSize},
		},
		{
			name: "caps page size",
			in:   PageParams{Page: 2, PageSize: 500},
			want: PageParams{Page: 2, PageSize: MaxPageSize},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Normalize()
			if got != tt.want {
				t.Fatalf("Normalize() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPageParamsOffsetLimit(t *testing.T) {
	p := PageParams{Page: 3, PageSize: 10}.Normalize()
	if p.Offset() != 20 {
		t.Fatalf("Offset() = %d, want 20", p.Offset())
	}
	if p.Limit() != 10 {
		t.Fatalf("Limit() = %d, want 10", p.Limit())
	}
}

func TestNewMeta(t *testing.T) {
	meta := NewMeta(1, 20, 45)
	if meta.Total != 45 || meta.TotalPages != 3 {
		t.Fatalf("NewMeta() = %+v, want total=45 total_pages=3", meta)
	}
}
