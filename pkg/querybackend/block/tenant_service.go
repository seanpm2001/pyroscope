package block

import (
	"bytes"
	"context"
	"fmt"

	"github.com/grafana/dskit/multierror"
	"github.com/parquet-go/parquet-go"
	"golang.org/x/sync/errgroup"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
	"github.com/grafana/pyroscope/pkg/phlaredb"
	schemav1 "github.com/grafana/pyroscope/pkg/phlaredb/schemas/v1"
	"github.com/grafana/pyroscope/pkg/phlaredb/symdb"
	"github.com/grafana/pyroscope/pkg/phlaredb/tsdb/index"
	"github.com/grafana/pyroscope/pkg/util"
	"github.com/grafana/pyroscope/pkg/util/refctr"
)

type Block interface {
	ProfileRowReader() parquet.RowReader
	Profiles() *ParquetFile
	Index() phlaredb.IndexReader
	Symbols() symdb.SymbolsReader
	Close() error
}

var _ Block = (*TenantService)(nil)

type TenantService struct {
	meta *metastorev1.TenantService
	obj  *Object

	refs refctr.Counter
	buf  *bytes.Buffer
	err  error

	tsdb     *index.Reader
	symbols  *symdb.Reader
	profiles *ParquetFile
}

func NewTenantService(meta *metastorev1.TenantService, obj *Object) *TenantService {
	return &TenantService{
		meta: meta,
		obj:  obj,
	}
}

func (s *TenantService) OpenShared(ctx context.Context, sections ...Section) (err error) {
	s.err = s.refs.Inc(func() error {
		return s.Open(ctx, sections...)
	})
	return s.err
}

func (s *TenantService) Open(ctx context.Context, sections ...Section) (err error) {
	if s.err != nil {
		// The tenant service has been already closed with an error.
		return s.err
	}
	if err = s.obj.OpenShared(ctx); err != nil {
		return fmt.Errorf("failed to open object: %w", err)
	}
	defer func() {
		// Close the object here because the tenant service won't be
		// closed if it fails to open.
		if err != nil {
			s.obj.CloseShared(err)
		}
	}()
	if s.obj.buf == nil && s.meta.Size < loadInMemorySizeThreshold {
		s.buf = new(bytes.Buffer) // TODO: Pool.
		off, size := int64(s.offset()), int64(s.meta.Size)
		if err = objstore.FetchRange(ctx, s.buf, s.obj.path, s.obj.storage, off, size); err != nil {
			return fmt.Errorf("loading sections into memory: %w", err)
		}
	}
	g, ctx := errgroup.WithContext(ctx)
	for _, sc := range sections {
		sc := sc
		g.Go(util.RecoverPanic(func() error {
			if err = sc.open(ctx, s); err != nil {
				return fmt.Errorf("openning section %v: %w", s.sectionName(sc), err)
			}
			return nil
		}))
	}
	return g.Wait()
}

func (s *TenantService) estimatedInMemorySizeMerge() int64 {
	var e int64
	// Both the symbols and the tsdb are loaded into memory entirely.
	// However, the actual footprint is higher because the encoding.
	e += s.sectionSize(SectionSymbols) * 4
	e += s.sectionSize(SectionTSDB) * 4
	// All columns are to be opened.
	columns := len(schemav1.ProfilesSchema.Columns())
	cb := estimateReadBufferSize(s.sectionSize(SectionProfiles))
	e += int64(columns * cb)
	return e
}

func (s *TenantService) CloseShared(err error) {
	s.refs.Dec(func() {
		s.closeErr(err)
	})
}

func (s *TenantService) Close() error {
	s.closeErr(nil)
	return s.err
}

func (s *TenantService) closeErr(err error) {
	if s.buf != nil {
		s.buf = nil // TODO: Release.
	}
	var m multierror.MultiError
	m.Add(s.err) // Preserve the existing error
	m.Add(err)   // Add the new error, if any.
	if s.tsdb != nil {
		m.Add(s.tsdb.Close())
	}
	if s.symbols != nil {
		m.Add(s.symbols.Close())
	}
	if s.profiles != nil {
		m.Add(s.profiles.Close())
	}
	s.err = m.Err()
}

func (s *TenantService) Meta() *metastorev1.TenantService { return s.meta }

func (s *TenantService) Profiles() *ParquetFile { return s.profiles }

func (s *TenantService) ProfileRowReader() parquet.RowReader { return s.profiles.RowReader() }

func (s *TenantService) Symbols() symdb.SymbolsReader { return s.symbols }

func (s *TenantService) Index() phlaredb.IndexReader { return s.tsdb }

// Offset of the tenant service section within the object.
func (s *TenantService) offset() uint64 { return s.meta.TableOfContents[0] }

func (s *TenantService) sectionIndex(sc Section) int {
	var n []int
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionIndices[1]
	}
	if int(sc) >= len(n) {
		panic(fmt.Sprintf("bug: invalid section index: %d (total: %d)", sc, len(n)))
	}
	return n[sc]
}

func (s *TenantService) sectionName(sc Section) string {
	var n []string
	switch s.obj.meta.FormatVersion {
	default:
		n = sectionNames[1]
	}
	if int(sc) >= len(n) {
		panic(fmt.Sprintf("bug: invalid section index: %d (total: %d)", sc, len(n)))
	}
	return n[sc]
}

func (s *TenantService) sectionOffset(sc Section) int64 {
	return int64(s.meta.TableOfContents[s.sectionIndex(sc)])
}

func (s *TenantService) sectionSize(sc Section) int64 {
	idx := s.sectionIndex(sc)
	off := s.meta.TableOfContents[idx]
	var next uint64
	if idx == len(s.meta.TableOfContents)-1 {
		next = s.offset() + s.meta.Size
	} else {
		next = s.meta.TableOfContents[idx+1]
	}
	return int64(next - off)
}

func (s *TenantService) inMemoryBuffer() []byte {
	if s.obj.buf != nil {
		// If the entire object is loaded into memory,
		// return the tenant service sub-slice.
		lo := s.offset()
		hi := lo + s.meta.Size
		buf := s.obj.buf.Bytes()
		return buf[lo:hi]
	}
	if s.buf != nil {
		// Otherwise, if the tenant service is loaded into memory
		// individually, return the buffer.
		return s.buf.Bytes()
	}
	// Otherwise, the tenant service is not loaded into memory.
	return nil
}

func (s *TenantService) inMemoryBucket(buf []byte) objstore.Bucket {
	bucket := memory.NewInMemBucket()
	bucket.Set(s.obj.path, buf)
	return objstore.NewBucket(bucket)
}
