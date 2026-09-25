package api

import (
	"strconv"

	"gravityreduce/internal/survey"
	"gravityreduce/internal/validate"
)

// fieldErrAPI 构造与既有风格一致的字段错误（422 由 abortFieldError 翻译）。
func fieldErrAPI(field, reason string) error {
	return validate.FieldError{Field: field, Reason: reason}
}

func itoa(i int) string { return strconv.Itoa(i) }

func reasonToView(r survey.Reason) *reasonView {
	if r.Code == "" {
		return nil
	}
	return &reasonView{Code: r.Code, Message: r.Message}
}

func buildLinesResponse(rep survey.Report) linesResponse {
	out := linesResponse{
		Lines: make([]lineView, 0, len(rep.Lines)),
		Pairs: make([]pairView, 0, len(rep.Pairs)),
	}

	for _, l := range rep.Lines {
		lv := lineView{
			ID:             l.ID,
			Status:         l.Status,
			Points:         make([]linePointView, 0, len(l.Points)),
			DegradeReasons: make([]reasonView, 0, len(l.DegradeReasons)),
		}
		for _, r := range l.DegradeReasons {
			lv.DegradeReasons = append(lv.DegradeReasons, reasonView{Code: r.Code, Message: r.Message})
		}
		if len(lv.DegradeReasons) == 0 {
			lv.DegradeReasons = nil
		}

		for _, p := range l.Points {
			pv := linePointView{
				ID:        p.ID,
				EastingM:  roundCoord(p.EastingM),
				NorthingM: roundCoord(p.NorthingM),
				HM:        roundMGal(p.HM),
				BouguerAnomaly: gravityView{
					MGal: roundMGal(p.BouguerAnomalyMGal),
					MS2:  roundMS2(p.BouguerAnomalyMS2),
				},
			}
			if p.HasStation {
				station := roundCoord(p.StationM)
				pv.StationM = &station
			}
			lv.Points = append(lv.Points, pv)
		}

		g := l.Gradient
		gv := gradientView{
			Status:          g.Status,
			SkipReason:      reasonToView(g.SkipReason),
			Segments:        make([]segmentView, 0, len(g.Segments)),
			SuspiciousCount: g.SuspiciousCount,
			CoincidentCount: g.CoincidentCount,
		}
		for _, s := range g.Segments {
			gv.Segments = append(gv.Segments, segmentView{
				FromID:              s.FromID,
				ToID:                s.ToID,
				FromStationM:        s.FromStationM,
				ToStationM:          s.ToStationM,
				HorizontalDistanceM: roundCoord(s.HorizontalDistanceM),
				AnomalyDeltaMGal:    roundMGal(s.AnomalyDeltaMGal),
				GradientMGalPerKm:   roundMGal(s.GradientMGalPerKm),
				ThresholdMGalPerKm:  roundMGal(s.ThresholdMGalPerKm),
				ExcessMGalPerKm:     roundMGal(s.ExcessMGalPerKm),
				Suspicious:          s.Suspicious,
				Kind:                s.Kind,
			})
		}
		lv.Gradient = gv

		st := l.Statistics
		lv.Statistics = statisticsView{
			PointCount: st.PointCount,
			MeanMGal:   roundMGal(st.MeanMGal),
			StdMGal:    roundMGal(st.StdMGal),
			Min:        extremumView{PointID: st.Min.PointID, MGal: roundMGal(st.Min.ValueMGal)},
			Max:        extremumView{PointID: st.Max.PointID, MGal: roundMGal(st.Max.ValueMGal)},
		}

		out.Lines = append(out.Lines, lv)
	}

	for _, pr := range rep.Pairs {
		pv := pairView{
			LineAID: pr.LineAID,
			LineBID: pr.LineBID,
			Status:  pr.Status,
			Reason:  reasonToView(pr.Reason),
		}
		if pr.Status == survey.StatusIntersected {
			e := roundCoord(pr.EastingM)
			n := roundCoord(pr.NorthingM)
			diff := roundMGal(pr.DifferenceMGal)
			tol := roundMGal(pr.ToleranceMGal)
			exceeds := pr.ExceedsTolerance
			pv.EastingM = &e
			pv.NorthingM = &n
			pv.A = sideToView(pr.A)
			pv.B = sideToView(pr.B)
			pv.DifferenceMGal = &diff
			pv.ToleranceMGal = &tol
			pv.ExceedsTolerance = &exceeds
		}
		out.Pairs = append(out.Pairs, pv)
	}
	return out
}

func sideToView(s *survey.InterpolatedSide) *interpolatedSideView {
	if s == nil {
		return nil
	}
	return &interpolatedSideView{
		LineID:        s.LineID,
		SegmentFromID: s.SegmentFromID,
		SegmentToID:   s.SegmentToID,
		ParameterT:    roundCoord(s.ParameterT),
		AnomalyMGal:   roundMGal(s.AnomalyMGal),
	}
}

// roundCoord 把坐标/距离/插值参数规整到 6 位小数（亚微米级），
// 避免 JSON 出现二进制浮点尾差；不参与任何物理计算，仅用于呈现。
func roundCoord(x float64) float64 { return roundMGal(x) }
