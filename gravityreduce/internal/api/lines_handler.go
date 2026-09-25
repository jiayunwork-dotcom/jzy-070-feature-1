package api

import (
	"errors"
	"fmt"
	"math"
	"net/http"

	"github.com/gin-gonic/gin"

	"gravityreduce/internal/gravity"
	"gravityreduce/internal/survey"
	"gravityreduce/internal/validate"
)

// linesResponse 是批量测线归算与质控响应的完整视图。
type linesResponse struct {
	Thresholds linesThresholdsView `json:"thresholds"`
	Lines      []lineResultView    `json:"lines"`
	Pairs      []pairResultView    `json:"pairs"`
}

type linesThresholdsView struct {
	GradientMGalPerM    float64 `json:"gradient_mgal_per_m"`
	IntersectionTolMGal float64 `json:"intersection_tolerance_mgal"`
}

type lineResultView struct {
	ID            string          `json:"id"`
	Status        string          `json:"status"`
	OrderingBasis string          `json:"ordering_basis"`
	Notes         []string        `json:"notes"`
	Points        []linePointView `json:"points"`
	Gradient      gradientView    `json:"gradient"`
	Statistics    statisticsView  `json:"statistics"`
}

type linePointView struct {
	ID                        string      `json:"id"`
	Order                     int         `json:"order"`
	EastingM                  float64     `json:"easting_m"`
	NorthingM                 float64     `json:"northing_m"`
	StationM                  float64     `json:"station_m,omitempty"`
	NormalGravity             gravityView `json:"normal_gravity"`
	FreeAirCorrectionMGal     float64     `json:"free_air_correction_mgal"`
	BouguerSlabCorrectionMGal float64     `json:"bouguer_slab_correction_mgal"`
	BouguerAnomaly            gravityView `json:"bouguer_anomaly"`
}

type gradientView struct {
	Available      bool                  `json:"available"`
	ThresholdMGalM float64               `json:"threshold_mgal_per_m"`
	Segments       []gradientSegmentView `json:"segments"`
	DataAnomalies  []zeroDistanceView    `json:"data_anomalies"`
}

type gradientSegmentView struct {
	FromPointID       string  `json:"from_point_id"`
	ToPointID         string  `json:"to_point_id"`
	FromStationM      float64 `json:"from_station_m"`
	ToStationM        float64 `json:"to_station_m"`
	DistanceM         float64 `json:"distance_m"`
	GradientMGalPerM  float64 `json:"gradient_mgal_per_m"`
	Suspicious        bool    `json:"suspicious"`
	ExceedsByMGalPerM float64 `json:"exceeds_by_mgal_per_m"`
}

type zeroDistanceView struct {
	LineID       string  `json:"line_id"`
	FromPointID  string  `json:"from_point_id"`
	ToPointID    string  `json:"to_point_id"`
	FromStationM float64 `json:"from_station_m"`
	ToStationM   float64 `json:"to_station_m"`
	Reason       string  `json:"reason"`
}

type statisticsView struct {
	Count      int     `json:"count"`
	MeanMGal   float64 `json:"mean_mgal"`
	StdMGal    float64 `json:"std_mgal"`
	MinMGal    float64 `json:"min_mgal"`
	MinPointID string  `json:"min_point_id"`
	MaxMGal    float64 `json:"max_mgal"`
	MaxPointID string  `json:"max_point_id"`
}

type pairResultView struct {
	LineAID      string         `json:"line_a_id"`
	LineBID      string         `json:"line_b_id"`
	Status       string         `json:"status"`
	Reason       string         `json:"reason,omitempty"`
	ReasonDetail string         `json:"reason_detail,omitempty"`
	Crossings    []crossingView `json:"crossings"`
}

type crossingView struct {
	EastingM     float64 `json:"easting_m"`
	NorthingM    float64 `json:"northing_m"`
	LineASegment struct {
		FromPointID string `json:"from_point_id"`
		ToPointID   string `json:"to_point_id"`
	} `json:"line_a_segment"`
	LineBSegment struct {
		FromPointID string `json:"from_point_id"`
		ToPointID   string `json:"to_point_id"`
	} `json:"line_b_segment"`
	InterpAMGal     float64 `json:"interp_a_mgal"`
	InterpBMGal     float64 `json:"interp_b_mgal"`
	DifferenceMGal  float64 `json:"difference_mgal"`
	ToleranceMGal   float64 `json:"tolerance_mgal"`
	WithinTolerance bool    `json:"within_tolerance"`
}

// ReduceLines 处理 POST /api/v1/lines/reduce：
// 一条或多条测线的批量归算 + 沿测线梯度检查 + 测线对交点差 + 线内统计。
func ReduceLines(c *gin.Context) {
	var req linesRequest
	if err := decodeStrict(c, &req); err != nil {
		abortError(c, http.StatusBadRequest, "", "请求体不是合法 JSON："+err.Error())
		return
	}

	var th survey.Thresholds
	var err error
	if th.GradientMGalPerM, err = requireField(req.GradientThresholdMGalPerM, "gradient_threshold_mgal_per_m"); err != nil {
		abortFieldError(c, err)
		return
	}
	if th.IntersectionTolMGal, err = requireField(req.IntersectionTolMGal, "intersection_tolerance_mgal"); err != nil {
		abortFieldError(c, err)
		return
	}

	lines, err := parseLines(req)
	if err != nil {
		abortFieldError(c, err)
		return
	}

	if err := survey.Validate(lines, th); err != nil {
		var fe survey.FieldError
		if errors.As(err, &fe) {
			abortError(c, http.StatusUnprocessableEntity, fe.Field, fe.Reason)
			return
		}
		abortError(c, http.StatusBadRequest, "", err.Error())
		return
	}

	res := survey.Process(lines, th)
	c.JSON(http.StatusOK, buildLinesResponse(res))
}

func parseLines(req linesRequest) ([]survey.Line, error) {
	out := make([]survey.Line, 0, len(req.Lines))
	for li, l := range req.Lines {
		lpf := fmt.Sprintf("lines[%d].points", li)
		sl := survey.Line{ID: l.ID, Points: make([]survey.Point, 0, len(l.Points))}
		for pi, p := range l.Points {
			pf := fmt.Sprintf("%s[%d]", lpf, pi)
			require := func(v *float64, name string) (float64, error) {
				val, err := requireField(v, name)
				if err != nil {
					var fe validate.FieldError
					if errors.As(err, &fe) {
						return 0, validate.FieldError{Field: pf + "." + fe.Field, Reason: fe.Reason}
					}
					return 0, err
				}
				return val, nil
			}
			east, err := require(p.Easting, "easting_m")
			if err != nil {
				return nil, err
			}
			north, err := require(p.Northing, "northing_m")
			if err != nil {
				return nil, err
			}
			gobs, err := require(p.Gobs, "gobs")
			if err != nil {
				return nil, err
			}
			h, err := require(p.H, "h")
			if err != nil {
				return nil, err
			}
			phi, err := require(p.Phi, "phi")
			if err != nil {
				return nil, err
			}
			rho, err := require(p.Rho, "rho")
			if err != nil {
				return nil, err
			}
			sl.Points = append(sl.Points, survey.Point{
				ID:       p.ID,
				Easting:  east,
				Northing: north,
				Station:  p.Station,
				Obs: gravity.Observation{
					GobsMS2: gobs,
					H:       h,
					Phi:     phi,
					Rho:     rho,
				},
			})
		}
		out = append(out, sl)
	}
	return out, nil
}

func buildLinesResponse(res survey.BatchResult) linesResponse {
	resp := linesResponse{
		Thresholds: linesThresholdsView{
			GradientMGalPerM:    res.Thresholds.GradientMGalPerM,
			IntersectionTolMGal: res.Thresholds.IntersectionTolMGal,
		},
		Lines: make([]lineResultView, 0, len(res.Lines)),
		Pairs: make([]pairResultView, 0, len(res.Pairs)),
	}

	for _, lr := range res.Lines {
		v := lineResultView{
			ID:            lr.LineID,
			Status:        lr.Status,
			OrderingBasis: lr.OrderingBasis,
			Notes:         lr.Notes,
			Points:        []linePointView{},
			Gradient: gradientView{
				Available:      lr.Gradient.Available,
				ThresholdMGalM: lr.Gradient.ThresholdMGalPerM,
				Segments:       []gradientSegmentView{},
				DataAnomalies:  []zeroDistanceView{},
			},
			Statistics: statisticsView{
				Count:      lr.Stats.Count,
				MeanMGal:   roundMGal(lr.Stats.MeanMGal),
				StdMGal:    roundMGal(lr.Stats.StdMGal),
				MinMGal:    roundMGal(lr.Stats.MinMGal),
				MinPointID: lr.Stats.MinPointID,
				MaxMGal:    roundMGal(lr.Stats.MaxMGal),
				MaxPointID: lr.Stats.MaxPointID,
			},
		}
		if len(lr.Notes) == 0 {
			v.Notes = []string{}
		}
		for _, p := range lr.Ordered {
			pv := linePointView{
				ID:                        p.Point.ID,
				Order:                     p.Order,
				EastingM:                  p.Point.Easting,
				NorthingM:                 p.Point.Northing,
				NormalGravity:             toGravityView(p.Result.NormalGravityMGal, p.Result.NormalGravityMS2),
				FreeAirCorrectionMGal:     roundMGal(p.Result.FreeAirMGal),
				BouguerSlabCorrectionMGal: roundMGal(p.Result.BouguerSlabMGal),
				BouguerAnomaly:            toGravityView(p.Result.BouguerAnomalyMGal, p.Result.BouguerAnomalyMS2),
			}
			if p.Point.Station != nil {
				pv.StationM = *p.Point.Station
			}
			v.Points = append(v.Points, pv)
		}
		for _, s := range lr.Gradient.Segments {
			v.Gradient.Segments = append(v.Gradient.Segments, gradientSegmentView{
				FromPointID:       s.FromPointID,
				ToPointID:         s.ToPointID,
				FromStationM:      s.FromStation,
				ToStationM:        s.ToStation,
				DistanceM:         roundM(s.DistanceM),
				GradientMGalPerM:  roundGradient(s.GradientMGalPerM),
				Suspicious:        s.Suspicious,
				ExceedsByMGalPerM: roundGradient(s.ExceedsByMGalPerM),
			})
		}
		for _, za := range lr.Gradient.DataAnomalies {
			v.Gradient.DataAnomalies = append(v.Gradient.DataAnomalies, zeroDistanceView{
				LineID:       za.LineID,
				FromPointID:  za.FromPointID,
				ToPointID:    za.ToPointID,
				FromStationM: za.FromStation,
				ToStationM:   za.ToStation,
				Reason:       za.Reason,
			})
		}
		resp.Lines = append(resp.Lines, v)
	}

	for _, pr := range res.Pairs {
		pv := pairResultView{
			LineAID:      pr.LineAID,
			LineBID:      pr.LineBID,
			Status:       pr.Status,
			Reason:       pr.Reason,
			ReasonDetail: pr.ReasonDetail,
			Crossings:    []crossingView{},
		}
		for _, cr := range pr.Crossings {
			cv := crossingView{
				EastingM:        roundM(cr.Easting),
				NorthingM:       roundM(cr.Northing),
				InterpAMGal:     roundMGal(cr.InterpAMGal),
				InterpBMGal:     roundMGal(cr.InterpBMGal),
				DifferenceMGal:  roundMGal(cr.DifferenceMGal),
				ToleranceMGal:   cr.ToleranceMGal,
				WithinTolerance: cr.WithinTolerance,
			}
			cv.LineASegment.FromPointID = cr.LineASegmentFromPointID
			cv.LineASegment.ToPointID = cr.LineASegmentToPointID
			cv.LineBSegment.FromPointID = cr.LineBSegmentFromPointID
			cv.LineBSegment.ToPointID = cr.LineBSegmentToPointID
			pv.Crossings = append(pv.Crossings, cv)
		}
		resp.Pairs = append(resp.Pairs, pv)
	}
	return resp
}

// roundGradient 把 mGal/m 规整到 9 位小数（亚 nGal/m 级），避免二进制尾差。
func roundGradient(x float64) float64 {
	return math.Round(x*1e9) / 1e9
}

// roundM 把米规整到 6 位小数。
func roundM(x float64) float64 {
	return math.Round(x*1e6) / 1e6
}
