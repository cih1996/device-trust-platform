package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCellDB(t *testing.T, rows ...string) *CellDB {
	t.Helper()
	file := filepath.Join(t.TempDir(), "460.csv")
	os.WriteFile(file, []byte("radio,mcc,net,area,cell,unit,lon,lat,range,samples,changeable,created,updated,averageSignal\n"+strings.Join(rows, "\n")+"\n"), 0600)
	db, err := LoadCellDB(file)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCellDBLookupAndChecks(t *testing.T) {
	db := testCellDB(t,
		"LTE,460,0,1,2,0,116.40,39.90,1000,20,1,1,1760000000,0",  // Beijing
		"UMTS,460,1,7,9,0,121.47,31.23,1000,20,1,1,1760000000,0", // Shanghai
		"NR,460,11,3,4,0,113.26,23.13,1000,1,1,1,1760000000,0",   // one sample
	)
	if db.Size() != 3 {
		t.Fatal(db.Size())
	}
	// SDK names WCDMA and pads the MNC.
	if _, ok := db.Lookup(cellObservation{Radio: "WCDMA", MCC: "460", MNC: "01", Area: 7, CellID: 9}); !ok {
		t.Fatal("WCDMA/01 lookup")
	}
	square := func(lon, lat float64) [][][]float64 {
		return [][][]float64{{{lon - 1, lat - 1}, {lon + 1, lat - 1}, {lon + 1, lat + 1}, {lon - 1, lat + 1}, {lon - 1, lat - 1}}}
	}
	engine := &ResourceEngine{Catalog: ResourceCatalog{Version: "TEST", Regions: []RegionBoundary{
		{Province: "北京市", Polygons: [][][][]float64{square(116.4, 39.9)}},
		{Province: "上海市", Polygons: [][][][]float64{square(121.47, 31.23)}},
	}}, Cells: db}
	age := 100.0
	accuracy := 20.0
	run := func(cell cellObservation, gps gpsObservation, fresh bool, transport string, ip *IPRegion) map[string]ResourceCheck {
		r := Report{Signals: map[string]Signal{"transport": {Status: "available", Value: transport}}}
		out := map[string]ResourceCheck{}
		engine.cellSiteChecks(r, []cellObservation{cell}, gps, fresh, ip, func(code, state, dimension, detail, source string, points int) {
			out[code] = ResourceCheck{Code: code, State: state, Detail: detail, Points: points}
		})
		return out
	}
	beijing := cellObservation{Radio: "LTE", MCC: "460", MNC: "00", Area: 1, CellID: 2, Registered: true, Age: &age}
	shanghaiGPS := gpsObservation{Latitude: 31.23, Longitude: 121.47, Accuracy: &accuracy}
	got := run(beijing, shanghaiGPS, true, "wifi", &IPRegion{CountryCode: "CN", Province: "上海市"})
	if got["cell_gps_mismatch"].Points != 25 || got["cell_ip_region_mismatch"].Points != 15 {
		t.Fatal(got)
	}
	// Mobile data egress is not compared with the cell's province.
	if got = run(beijing, shanghaiGPS, false, "cellular", &IPRegion{CountryCode: "CN", Province: "上海市"}); len(got) != 0 {
		t.Fatal(got)
	}
	nearby := gpsObservation{Latitude: 39.95, Longitude: 116.45, Accuracy: &accuracy}
	if got = run(beijing, nearby, true, "wifi", &IPRegion{CountryCode: "CN", Province: "北京市"}); got["cell_gps_crosscheck"].State != "matched" || got["cell_ip_region"].State != "matched" {
		t.Fatal(got)
	}
	thin := cellObservation{Radio: "NR", MCC: "460", MNC: "11", Area: 3, CellID: 4, Registered: true, Age: &age}
	if got = run(thin, shanghaiGPS, true, "wifi", nil); got["cell_location"].State != "stale" || got["cell_gps_mismatch"].Points != 0 {
		t.Fatal(got)
	}
	unknown := cellObservation{Radio: "LTE", MCC: "460", MNC: "00", Area: 1, CellID: 99, Registered: true, Age: &age}
	if got = run(unknown, shanghaiGPS, true, "wifi", nil); got["cell_location"].State != "unknown" {
		t.Fatal(got)
	}
	// The test handset's radio reports TAC 1234 / Cell ID 5678.
	fake := cellObservation{Radio: "LTE", MCC: "460", MNC: "00", Area: 1234, CellID: 5678, Registered: true, Age: &age}
	if got = run(fake, shanghaiGPS, false, "cellular", nil); got["cell_identity_implausible"].Points != 25 {
		t.Fatal(got)
	}
	real := cellObservation{Radio: "LTE", MCC: "460", MNC: "00", Area: 22836, CellID: 123456819 + 7, Registered: true, Age: &age}
	if got = run(real, shanghaiGPS, false, "cellular", nil); got["cell_identity_implausible"].Points != 0 {
		t.Fatal(got)
	}
	if _, err := LoadCellDB(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Fatal("missing file accepted")
	}
	bad := filepath.Join(t.TempDir(), "bad.csv")
	os.WriteFile(bad, []byte("LTE,460,0,1\n"), 0600)
	if _, err := LoadCellDB(bad); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatal(err)
	}
}

func TestCellDBSnapshot(t *testing.T) {
	file := os.Getenv("DTP_CELL_DB")
	if file == "" {
		t.Skip("optional OpenCelliD snapshot test")
	}
	db, err := LoadCellDB(file)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d cells, newest %s", db.Source, db.Size(), db.Updated)
}

func TestCellCluster(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	now := time.Now().UTC()
	cell := func(id string, at time.Time) Report {
		return Report{ID: id, AppID: "a", DeviceID: id, LinkID: "L" + id, ReceivedAt: at, Resources: &ResourceAssessment{}, Signals: map[string]Signal{
			"cell": {Status: "available", Value: []any{map[string]any{"technology": "LTE", "mcc": "460", "mnc": "00", "areaCode": 22836, "cellId": 123456819, "registered": true, "ageMillis": 500}}}}}
	}
	for i := 0; i < 9; i++ {
		store.put("reports", fmt.Sprint("r", i), cell(fmt.Sprint("r", i), now.Add(-30*time.Minute)))
	}
	store.put("reports", "old", cell("old", now.Add(-2*time.Hour)))
	r := cell("self", now)
	if err := s.cellCluster(&r); err != nil || len(r.Resources.Checks) != 1 || r.Resources.Checks[0].Points != 15 {
		t.Fatal(r.Resources.Checks, err)
	}
	r = cell("r8", now) // a handset already counted adds nothing
	r.Resources = &ResourceAssessment{}
	if err := s.cellCluster(&r); err != nil || len(r.Resources.Checks) != 0 {
		t.Fatal(r.Resources.Checks)
	}
}
