package querybackend

import (
	"sync"

	"github.com/prometheus/prometheus/model/labels"

	querybackendv1 "github.com/grafana/pyroscope/api/gen/proto/go/querybackend/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/querybackend/block"
)

func init() {
	registerQueryType(
		querybackendv1.QueryType_QUERY_SERIES_LABELS,
		querybackendv1.ReportType_REPORT_SERIES_LABELS,
		querySeriesLabels,
		newSeriesLabelsAggregator,
		[]block.Section{block.SectionTSDB}...,
	)
}

func querySeriesLabels(q *queryContext, query *querybackendv1.Query) (*querybackendv1.Report, error) {
	postings, err := getPostings(q.svc.Index(), q.req.matchers...)
	if err != nil {
		return nil, err
	}
	var tmp model.Labels
	var c []index.ChunkMeta
	l := make(map[uint64]model.Labels)
	for postings.Next() {
		fp, _ := q.svc.Index().SeriesBy(postings.At(), &tmp, &c, query.SeriesLabels.LabelNames...)
		if _, ok := l[fp]; ok {
			continue
		}
		l[fp] = tmp.Clone()
	}
	if err = postings.Err(); err != nil {
		return nil, err
	}
	series := make([]*typesv1.Labels, len(l))
	var i int
	for _, s := range l {
		series[i] = &typesv1.Labels{Labels: s}
		i++
	}
	resp := &querybackendv1.Report{
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query:        query.SeriesLabels.CloneVT(),
			SeriesLabels: series,
		},
	}
	return resp, nil
}

func getPostings(reader phlaredb.IndexReader, matchers ...*labels.Matcher) (index.Postings, error) {
	if len(matchers) == 0 {
		k, v := index.AllPostingsKey()
		return reader.Postings(k, nil, v)
	}
	return phlaredb.PostingsForMatchers(reader, nil, matchers...)
}

type seriesLabelsAggregator struct {
	init   sync.Once
	query  *querybackendv1.SeriesLabelsQuery
	series *model.LabelMerger
}

func newSeriesLabelsAggregator(*querybackendv1.InvokeRequest) aggregator {
	return new(seriesLabelsAggregator)
}

func (a *seriesLabelsAggregator) aggregate(report *querybackendv1.Report) error {
	r := report.SeriesLabels
	a.init.Do(func() {
		a.query = r.Query.CloneVT()
		a.series = model.NewLabelMerger()
	})
	a.series.MergeSeries(r.SeriesLabels)
	return nil
}

func (a *seriesLabelsAggregator) build() *querybackendv1.Report {
	return &querybackendv1.Report{
		SeriesLabels: &querybackendv1.SeriesLabelsReport{
			Query:        a.query,
			SeriesLabels: a.series.SeriesLabels(),
		},
	}
}
