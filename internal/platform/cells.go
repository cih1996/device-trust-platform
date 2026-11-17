package platform

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CellDB is a crowdsourced tower position table in OpenCelliD CSV format
// (radio,mcc,net,area,cell,unit,lon,lat,range,samples,changeable,created,
// updated,averageSignal), loaded whole: the per-country file for China is a
// few hundred thousand rows. Data © OpenCelliD, CC BY-SA 4.0.
type CellDB struct {
	Source  string
	Updated time.Time // newest measurement in the file
	cells   map[cellKey]CellSite
}

type cellKey struct {
	radio    string
	mcc, mnc int
	area     int64
	cell     int64
}

type CellSite struct {
	Latitude     float64
	Longitude    float64
	RadiusMeters float64
	Samples      int
	Updated      time.Time
}

// Positions averaged from fewer measurements are not compared.
const cellMinSamples = 3

// cellRadio maps the SDK's technology names to OpenCelliD's.
func cellRadio(technology string) string {
	switch strings.ToUpper(technology) {
	case "WCDMA", "UMTS":
		return "UMTS"
	case "GSM", "LTE", "NR", "CDMA":
		return strings.ToUpper(technology)
	}
	return ""
}

func LoadCellDB(path string) (*CellDB, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var input io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		z, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer z.Close()
		input = z
	}
	db := &CellDB{Source: "OpenCelliD " + filepath.Base(path), cells: map[cellKey]CellSite{}}
	scanner := bufio.NewScanner(input)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if line == 1 && strings.HasPrefix(text, "radio,") {
			continue
		}
		key, site, err := parseCellRow(text)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		db.cells[key] = site
		if site.Updated.After(db.Updated) {
			db.Updated = site.Updated
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(db.cells) == 0 {
		return nil, fmt.Errorf("%s: no cells", path)
	}
	return db, nil
}

func parseCellRow(text string) (cellKey, CellSite, error) {
	f := strings.Split(text, ",")
	if len(f) != 14 {
		return cellKey{}, CellSite{}, fmt.Errorf("expected 14 fields, got %d", len(f))
	}
	ints := make([]int64, 0, 8)
	for _, i := range []int{1, 2, 3, 4, 8, 9, 12} {
		v, err := strconv.ParseInt(f[i], 10, 64)
		if err != nil {
			return cellKey{}, CellSite{}, err
		}
		ints = append(ints, v)
	}
	lon, err := strconv.ParseFloat(f[6], 64)
	if err != nil {
		return cellKey{}, CellSite{}, err
	}
	lat, err := strconv.ParseFloat(f[7], 64)
	if err != nil {
		return cellKey{}, CellSite{}, err
	}
	radio := cellRadio(f[0])
	if radio == "" || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return cellKey{}, CellSite{}, fmt.Errorf("invalid radio or position")
	}
	key := cellKey{radio, int(ints[0]), int(ints[1]), ints[2], ints[3]}
	return key, CellSite{Latitude: lat, Longitude: lon, RadiusMeters: float64(ints[4]), Samples: int(ints[5]), Updated: time.Unix(ints[6], 0).UTC()}, nil
}

func (db *CellDB) Lookup(c cellObservation) (CellSite, bool) {
	if db == nil {
		return CellSite{}, false
	}
	mcc, err1 := strconv.Atoi(c.MCC)
	mnc, err2 := strconv.Atoi(c.MNC)
	if err1 != nil || err2 != nil {
		return CellSite{}, false
	}
	site, ok := db.cells[cellKey{cellRadio(c.Radio), mcc, mnc, c.Area, c.CellID}]
	return site, ok
}

func (db *CellDB) Size() int { return len(db.cells) }

// servingCells returns the registered cells measured in the last two minutes.
func servingCells(cells []cellObservation) []cellObservation {
	var out []cellObservation
	for _, c := range cells {
		if c.Registered && c.Age != nil && *c.Age >= 0 && *c.Age <= 120000 {
			out = append(out, c)
		}
	}
	return out
}

// cellSiteChecks places the serving cell with the tower table and compares it
// with GPS and, on Wi-Fi, with the broadband IP's province.
func (e *ResourceEngine) cellSiteChecks(r Report, cells []cellObservation, gps gpsObservation, fresh bool, ip *IPRegion, add func(code, state, dimension, detail, source string, points int)) {
	if e.Cells == nil {
		add("cell_location", "unknown", "network", "未加载基站库", e.Catalog.Version, 0)
		return
	}
	source := e.Cells.Source
	serving := servingCells(cells)
	if len(serving) == 0 {
		add("cell_location", "unknown", "network", "没有两分钟内的注册基站", source, 0)
		return
	}
	located := false
	for _, cell := range serving {
		site, ok := e.Cells.Lookup(cell)
		if !ok {
			if implausibleCell(cell) {
				add("cell_identity_implausible", "conflict", "network", fmt.Sprintf("注册基站 %s TAC/LAC %d、Cell ID %d 不在基站库中且像占位值", cell.Radio, cell.Area, cell.CellID), source, 25)
			}
			continue
		}
		located = true
		if site.Samples < cellMinSamples {
			add("cell_location", "stale", "network", fmt.Sprintf("基站库样本 %d 个，位置不足以比较", site.Samples), source, 0)
			continue
		}
		// Crowdsourced positions are averages of where phones heard the cell; a
		// macro cell reaches tens of kilometres in the countryside.
		if fresh && !gps.Mock {
			distance := distanceMeters(gps.Latitude, gps.Longitude, site.Latitude, site.Longitude)
			threshold := max(site.RadiusMeters*3, 20000) + *gps.Accuracy
			if distance > threshold {
				add("cell_gps_mismatch", "conflict", "network", fmt.Sprintf("定位与注册基站相距 %.0f km，比较半径 %.0f km", distance/1000, threshold/1000), source, 25)
			} else {
				add("cell_gps_crosscheck", "matched", "network", fmt.Sprintf("定位与注册基站相距 %.1f km", distance/1000), source, 0)
			}
		}
		e.cellIPRegion(r, site, ip, source, add)
	}
	if !located {
		add("cell_location", "unknown", "network", "基站库中没有当前注册基站", source, 0)
	}
}

// cellIPRegion compares the cell's province with a broadband egress. Mobile
// data egress is not compared: domestic roaming exits in the home province.
func (e *ResourceEngine) cellIPRegion(r Report, site CellSite, ip *IPRegion, source string, add func(code, state, dimension, detail, source string, points int)) {
	transport, _ := r.Signals["transport"].Value.(string)
	if ip == nil || ip.CountryCode != "CN" || ip.Province == "" || transport != "wifi" || r.Signals["vpn"].Value == true {
		return
	}
	province, border := locateRegion(site.Latitude, site.Longitude, e.Catalog.Regions)
	switch {
	case province == "" || border < 50000:
		return
	case normalizeProvince(province) == normalizeProvince(ip.Province):
		add("cell_ip_region", "matched", "network", "基站与宽带出口同在"+province, source, 0)
	default:
		add("cell_ip_region_mismatch", "conflict", "network", "基站在"+province+"，宽带出口在"+ip.Province, source, 15)
	}
}

// implausibleCell flags a Chinese LTE/NR serving cell whose identity looks
// typed in by a radio simulator: an LTE ECI or NR NCI below 65536 (real ones
// are eNB/gNB ID shifted left, 99.9% above that in OpenCelliD's China data),
// or a placeholder such as 1234, 5678 or 9999 in the cell or area code.
func implausibleCell(c cellObservation) bool {
	radio := cellRadio(c.Radio)
	if c.MCC != "460" || (radio != "LTE" && radio != "NR") {
		return false
	}
	return c.CellID < 65536 || placeholderNumber(c.CellID) || placeholderNumber(c.Area)
}

// placeholderNumber is a run of four or more identical or ascending digits.
func placeholderNumber(v int64) bool {
	s := strconv.FormatInt(v, 10)
	if len(s) < 4 {
		return false
	}
	same, ascending := true, true
	for i := 1; i < len(s); i++ {
		same = same && s[i] == s[0]
		ascending = ascending && s[i] == s[i-1]+1
	}
	return same || ascending
}

const (
	cellClusterWindow  = time.Hour
	cellClusterDevices = 10
)

func servingCellKey(r Report) string {
	var cells []cellObservation
	if signalObject(r, "cell", &cells) != nil {
		return ""
	}
	for _, c := range servingCells(cells) {
		if radio := cellRadio(c.Radio); radio != "" && c.CellID > 0 {
			return fmt.Sprintf("%s/%s/%s/%d/%d", radio, c.MCC, strings.TrimLeft(c.MNC, "0"), c.Area, c.CellID)
		}
	}
	return ""
}

// cellCluster counts handsets registered on the same cell within the last
// hour, across apps: phones racked in one room on their own SIMs share a cell.
// A busy downtown cell serves many users too, so the threshold is high.
func (s *Server) cellCluster(r *Report) error {
	key := servingCellKey(*r)
	if r.Resources == nil || key == "" {
		return nil
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return err
	}
	self := handset(*r)
	seen := map[string]bool{self: true}
	for _, other := range reports {
		age := r.ReceivedAt.Sub(other.ReceivedAt)
		if age < 0 || age > cellClusterWindow || seen[handset(other)] || servingCellKey(other) != key {
			continue
		}
		seen[handset(other)] = true
	}
	if len(seen) >= cellClusterDevices {
		r.Resources.Checks = append(r.Resources.Checks, ResourceCheck{Code: "cell_device_cluster", State: "conflict", Dimension: "network",
			Detail: fmt.Sprintf("1 小时内 %d 台手机注册在同一基站", len(seen)), Source: "client", Points: 15})
	}
	return nil
}
