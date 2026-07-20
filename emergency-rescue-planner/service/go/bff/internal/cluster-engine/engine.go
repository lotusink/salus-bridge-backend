package cluster_engine

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	conditions_engine "bff/internal/conditions-engine"
	field_report_engine "bff/internal/field-report-engine"
)

const (
	clusterProximityDeg = 0.005 // ~500 m grid cell in Melbourne latitude
	clusterTimeBucket   = 30 * time.Minute
	clusterThreshold    = 3      // 3+ confirms of same type/location/time → promote
	polygonRadius       = 0.0027 // radius in degrees of the synthesized zone polygon (~300 m at Melbourne latitude)
)

// ClusterEngine detects field-report clusters and promotes them to hazard zones.
// Implements field_report_engine.ClusterNotifier.
type ClusterEngine struct {
	reportStore field_report_engine.FieldReportStore
	cEngine     *conditions_engine.ConditionsEngine
	mu          sync.Mutex
	promoted    map[string]string // clusterID → promoted zone ID (idempotency)
}

func NewClusterEngine(
	reportStore field_report_engine.FieldReportStore,
	cEngine *conditions_engine.ConditionsEngine,
) *ClusterEngine {
	return &ClusterEngine{
		reportStore: reportStore,
		cEngine:     cEngine,
		promoted:    make(map[string]string),
	}
}

// OnReportConfirmed is called synchronously after POST /api/field-reports/{id}/confirm succeeds.
func (e *ClusterEngine) OnReportConfirmed(ctx context.Context, info field_report_engine.ConfirmedReportInfo) {
	clusterID := deriveClusterID(info.TypeID, info.Lat, info.Lng, info.CreatedAt)

	e.mu.Lock()
	if _, alreadyPromoted := e.promoted[clusterID]; alreadyPromoted {
		e.mu.Unlock()
		return
	}

	allReports := e.reportStore.GetConfirmedReportInfos()
	var clusterReports []field_report_engine.ConfirmedReportInfo
	for _, r := range allReports {
		if deriveClusterID(r.TypeID, r.Lat, r.Lng, r.CreatedAt) == clusterID {
			clusterReports = append(clusterReports, r)
		}
	}

	if len(clusterReports) < clusterThreshold {
		e.mu.Unlock()
		return
	}

	zone := synthesizeZone(clusterID, clusterReports)
	e.promoted[clusterID] = zone.ID
	e.mu.Unlock() // release before I/O (ActivateZone may trigger ORS calls via hook)

	_ = e.cEngine.ActivateZone(ctx, zone)
}

func deriveClusterID(typeID string, lat, lng float64, createdAt time.Time) string {
	latBucket := math.Round(lat/clusterProximityDeg) * clusterProximityDeg
	lngBucket := math.Round(lng/clusterProximityDeg) * clusterProximityDeg
	timeBucket := createdAt.Truncate(clusterTimeBucket).Unix()
	return fmt.Sprintf("%s:%.3f:%.3f:%d", typeID, latBucket, lngBucket, timeBucket)
}

func synthesizeZone(clusterID string, reports []field_report_engine.ConfirmedReportInfo) conditions_engine.Zone {
	var sumLat, sumLng float64
	for _, r := range reports {
		sumLat += r.Lat
		sumLng += r.Lng
	}
	centLat := sumLat / float64(len(reports))
	centLng := sumLng / float64(len(reports))

	// 8-point circle polygon approximation at polygonRadius degrees
	polygon := make([][2]float64, 8)
	for i := 0; i < 8; i++ {
		angle := float64(i) * (2 * math.Pi / 8)
		polygon[i] = [2]float64{
			centLat + polygonRadius*math.Sin(angle),
			centLng + polygonRadius*math.Cos(angle),
		}
	}

	typeID := reports[0].TypeID
	label := clusterTypeLabel(typeID)
	zoneID := "cluster-" + clusterID

	return conditions_engine.Zone{
		ID:          zoneID,
		Level:       conditions_engine.ZoneLevelHigh,
		Label:       "Cluster: " + label,
		Source:      "field-report-cluster",
		UpdatedAt:   time.Now(),
		ActiveAlert: true,
		Polygon:     polygon,
	}
}

func clusterTypeLabel(typeID string) string {
	parts := strings.Split(typeID, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
