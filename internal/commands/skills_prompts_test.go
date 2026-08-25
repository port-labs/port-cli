package commands

import "testing"

func TestInitMetadataCatalogQueryFetchesFullCatalog(t *testing.T) {
	query := initMetadataCatalogQuery()

	if query.TeamsDefault == nil || *query.TeamsDefault {
		t.Fatalf("TeamsDefault = %v, want false", query.TeamsDefault)
	}
	if !query.ExcludeFiles {
		t.Fatal("ExcludeFiles = false, want true")
	}
	if !query.IncludeUngrouped {
		t.Fatal("IncludeUngrouped = false, want true")
	}
	if len(query.Exclude) != 1 || query.Exclude[0] != "internal" {
		t.Fatalf("Exclude = %v, want [internal]", query.Exclude)
	}
}
