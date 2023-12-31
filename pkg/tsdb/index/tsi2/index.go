package tsi2

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/influxdata/influxdb/pkg/bytesutil"
	"github.com/influxdata/influxdb/v2/logger"
	"github.com/influxdata/influxdb/v2/models"
	"github.com/influxdata/influxdb/v2/pkg/estimator"
	"github.com/influxdata/influxql"
	"go.uber.org/zap"

	"cycledb/pkg/tsdb"
)

var (
	Version      = 1
	IndexFileExt = ".tsi2"

	// indexFileBufferSize is the buffer size used when compacting the LogFile down
	// into a .tsi file.
	indexFileBufferSize = 1 << 17 // 128K

	IndexFilePath = "./tmp"
)

type Index struct {
	// todo(vinland): partition

	measurements *Measurements

	logger *zap.Logger // Index's logger.

	// The following must be set when initializing an Index.
	sfile    *tsdb.SeriesFile // series lookup file
	database string           // Name of database.

	// todo(vinland): to estimate bytes
	// need to be protected by mutex
	// // Cached sketches.
	// mSketch, mTSketch estimator.Sketch // Measurement sketches, add, delete?
	// sSketch, sTSketch estimator.Sketch // Series sketches

	path string // Root directory of the index partitions.

	fieldSet *tsdb.MeasurementFieldSet

	// Index's version.
	version int

	opened bool
}

// An IndexOption is a functional option for changing the configuration of
// an Index.
type IndexOption func(i *Index)

// WithPath sets the root path of the Index
var WithPath = func(path string) IndexOption {
	return func(i *Index) {
		i.path = path
	}
}

// NewIndex returns a new instance of Index.
func NewIndex(sfile *tsdb.SeriesFile, database string, options ...IndexOption) *Index {
	idx := &Index{
		logger:   zap.NewNop(),
		version:  Version,
		sfile:    sfile,
		database: database,
	}

	for _, option := range options {
		option(idx)
	}

	return idx
}

func (i *Index) Open() error {
	if i.opened {
		return errors.New("index already open")
	}
	i.measurements = NewMeasurements()
	i.opened = true
	i.path = IndexFilePath
	return nil
}

func (i *Index) Close() error {
	i.opened = false
	return nil
}

func (i *Index) WithLogger(*zap.Logger) {}

func (i *Index) Database() string {
	return i.database
}

func (i *Index) MeasurementExists(name []byte) (bool, error) {
	// if _, ok := i.measurementToGIndexes[string(name[:])]; !ok {
	// 	return false, nil
	// } else {
	// 	return true, nil
	// }
	gIndex, err := i.measurements.MeasurementByName(name)
	if err != nil {
		return false, err
	}
	if gIndex == nil {
		return false, nil
	}
	return true, nil
}

func (i *Index) MeasurementNamesByRegex(re *regexp.Regexp) ([][]byte, error) {
	// return [][]byte{[]byte("measurement_test")}, nil
	var res [][]byte
	for m, _ := range i.measurements.measurementId {
		if re.MatchString(m) {
			// Clone bytes since they will be used after the fileset is released.
			res = append(res, bytesutil.Clone([]byte(m)))
		}
	}
	return res, nil
}

// SeriesFile returns the series file attached to the index.
func (i *Index) SeriesFile() *tsdb.SeriesFile { return i.sfile }

func (i *Index) DropMeasurement(name []byte) error {
	return i.measurements.DropMeasurement(name)
}

func (i *Index) ForEachMeasurementName(fn func(name []byte) error) error {
	for m := range i.measurements.measurementId {
		if err := fn([]byte(m)); err != nil {
			return err
		}
	}
	return nil
}

func (i *Index) CreateSeriesListIfNotExists(keys, names [][]byte, tagsSlice []models.Tags) error {
	if len(names) == 0 {
		return nil
	} else if len(names) != len(tagsSlice) {
		return fmt.Errorf("uneven batch, sent %d names and %d tags", len(names), len(tagsSlice))
	}
	newIDs := make([]uint64, 0)
	newNames := make([][]byte, 0)
	newTagsSlice := make([]models.Tags, 0)
	for index := range names {
		buf := make([]byte, 1024)
		// 1. check if this seriesKey already exists in seriesFile
		// todo(vinland): series file and index could have inconsistent series
		if exist := i.sfile.HasSeries(names[index], tagsSlice[index], buf); !exist {
			// 2. if not. add to grid index
			exist, err := i.MeasurementExists(names[index])
			if err != nil {
				return err
			}
			if !exist {
				i.measurements.AppendMeasurement(names[index])
			}
			id, success := i.measurements.SetTags(names[index], tagsSlice[index])
			if !success {
				continue
			}
			newIDs = append(newIDs, id)
			newTagsSlice = append(newTagsSlice, tagsSlice[index])
			newNames = append(newNames, names[index])
		}
	}

	// 3. add to seriesFile
	_, err := i.sfile.CreateSeriesListIfNotExistsWithDesignatedIDs(newNames, newTagsSlice, newIDs)
	if err != nil {
		return err
	}

	return nil
}

func (i *Index) CreateSeriesIfNotExists(key, name []byte, tags models.Tags) error {
	return i.CreateSeriesListIfNotExists([][]byte{name}, [][]byte{name}, []models.Tags{tags})
}

func (i *Index) DropSeries(seriesID uint64, key []byte, cascade bool) error {
	panic("unimplemented")
}
func (i *Index) DropMeasurementIfSeriesNotExist(name []byte) (bool, error) {
	panic("unimplemented")
}

// MeasurementsSketches returns the two measurement sketches for the index.
func (i *Index) MeasurementsSketches() (estimator.Sketch, estimator.Sketch, error) {
	// i.mu.RLock()
	// defer i.mu.RUnlock()
	// return i.mSketch.Clone(), i.mTSketch.Clone(), nil
	panic("not implemented")
}

// SeriesSketches returns the two series sketches for the index.
func (i *Index) SeriesSketches() (estimator.Sketch, estimator.Sketch, error) {
	// i.mu.RLock()
	// defer i.mu.RUnlock()
	// return i.sSketch.Clone(), i.sTSketch.Clone(), nil
	panic("not implemented")
}

func (i *Index) SeriesIDSet() *tsdb.SeriesIDSet {
	// return i.measurements.SeriesIDSet()
	panic("unimplemented")
}

func (i *Index) SeriesN() int64 {
	return int64(i.SeriesIDSet().Cardinality())
}

func (i *Index) HasTagKey(name, key []byte) (bool, error) {
	return i.measurements.HasTagKey(name, key)
}
func (i *Index) HasTagValue(name, key, value []byte) (bool, error) {
	return i.measurements.HasTagValue(name, key, value)
}

// MeasurementTagKeysByExpr extracts the tag keys wanted by the expression.
func (i *Index) MeasurementTagKeysByExpr(name []byte, expr influxql.Expr) (map[string]struct{}, error) {
	// Return all keys if no condition was passed in.
	// if expr == nil {
	// 	m := make(map[string]struct{})
	// 	if itr := i.TagKeyIterator(name); itr != nil {
	// 		for e := itr.Next(); e != nil; e = itr.Next() {
	// 			m[string(e.Key())] = struct{}{}
	// 		}
	// 	}
	// 	return m, nil
	// }

	// switch e := expr.(type) {
	// case *influxql.BinaryExpr:
	// 	switch e.Op {
	// 	case influxql.EQ, influxql.NEQ, influxql.EQREGEX, influxql.NEQREGEX:
	// 		tag, ok := e.LHS.(*influxql.VarRef)
	// 		if !ok {
	// 			return nil, fmt.Errorf("left side of '%s' must be a tag key", e.Op.String())
	// 		} else if tag.Val != "_tagKey" {
	// 			return nil, nil
	// 		}

	// 		if influxql.IsRegexOp(e.Op) {
	// 			re, ok := e.RHS.(*influxql.RegexLiteral)
	// 			if !ok {
	// 				return nil, fmt.Errorf("right side of '%s' must be a regular expression", e.Op.String())
	// 			}
	// 			return i.tagKeysByFilter(name, e.Op, nil, re.Val), nil
	// 		}

	// 		s, ok := e.RHS.(*influxql.StringLiteral)
	// 		if !ok {
	// 			return nil, fmt.Errorf("right side of '%s' must be a tag value string", e.Op.String())
	// 		}
	// 		return i.tagKeysByFilter(name, e.Op, []byte(s.Val), nil), nil

	// 	case influxql.AND, influxql.OR:
	// 		lhs, err := i.MeasurementTagKeysByExpr(name, e.LHS)
	// 		if err != nil {
	// 			return nil, err
	// 		}

	// 		rhs, err := i.MeasurementTagKeysByExpr(name, e.RHS)
	// 		if err != nil {
	// 			return nil, err
	// 		}

	// 		if lhs != nil && rhs != nil {
	// 			if e.Op == influxql.OR {
	// 				return unionStringSets(lhs, rhs), nil
	// 			}
	// 			return intersectStringSets(lhs, rhs), nil
	// 		} else if lhs != nil {
	// 			return lhs, nil
	// 		} else if rhs != nil {
	// 			return rhs, nil
	// 		}
	// 		return nil, nil
	// 	default:
	// 		return nil, fmt.Errorf("invalid operator for tag keys by expression")
	// 	}

	// case *influxql.ParenExpr:
	// 	return i.MeasurementTagKeysByExpr(name, e.Expr)
	// }

	// return nil, fmt.Errorf("invalid measurement tag keys expression: %#v", expr)
	panic("not implemented")
}

// TagKeyCardinality always returns zero.
// It is not possible to determine cardinality of tags across index files, and
// thus it cannot be done across partitions.
func (i *Index) TagKeyCardinality(name, key []byte) int {
	return 0
}

func (i *Index) MeasurementIterator() (tsdb.MeasurementIterator, error) {
	return NewMeasurementsIterator(i.measurements), nil
}

func (i *Index) TagKeyIterator(name []byte) (tsdb.TagKeyIterator, error) {
	m, err := i.measurements.MeasurementByName(name)
	if err != nil || m == nil {
		return nil, err
	}
	return NewTagKeyIterator(m.gIndex), nil
}

func (i *Index) TagValueIterator(name, key []byte) (tsdb.TagValueIterator, error) {
	m, err := i.measurements.MeasurementByName(name)
	if err != nil || m == nil {
		return nil, err
	}
	return NewTagValueIterator(m.gIndex, key), nil
}

func (i *Index) MeasurementSeriesIDIterator(name []byte) (tsdb.SeriesIDIterator, error) {
	return i.measurements.MeasurementSeriesIDIterator(name)
}

func (i *Index) TagKeySeriesIDIterator(name, key []byte) (tsdb.SeriesIDIterator, error) {
	return i.measurements.TagKeySeriesIDIterator(name, key)
}

func (i *Index) TagValueSeriesIDIterator(name, key, value []byte) (tsdb.SeriesIDIterator, error) {
	return i.measurements.TagValueSeriesIDIterator(name, key, value)
}

// Sets a shared fieldset from the engine.
func (i *Index) FieldSet() *tsdb.MeasurementFieldSet {
	return i.fieldSet
}
func (i *Index) SetFieldSet(fs *tsdb.MeasurementFieldSet) {
	i.fieldSet = fs
}

// Size of the index on disk, if applicable.
func (i *Index) DiskSizeBytes() int64 {
	panic("unimplemented")
}

// Bytes estimates the memory footprint of this Index, in bytes.
func (i *Index) Bytes() int {
	panic("unimplemented")
}

func (i *Index) Type() string {
	panic("unimplemented")
}

// Returns a unique reference ID to the index instance.
func (i *Index) UniqueReferenceID() uintptr {
	panic("unimplemented")
}

func (i *Index) Compact(id int) error {
	start := time.Now()

	log, logEnd := logger.NewOperation(context.TODO(), i.logger, "TSI2 compaction", "tsi2_compact", zap.Int("tsi2_id", id))
	defer logEnd()

	// Create new index file.
	path := filepath.Join(i.path, FormatIndexFileName(id, 1))
	f, err := os.Create(path)
	if err != nil {
		log.Error("Cannot create index file", zap.Error(err))
		return err
	}
	// defer f.Close()

	// Compact index in memory to new index file.
	// lvl := tsi1.CompactionLevel{M: 1 << 25, K: 6}
	n, err := i.CompactTo(f)
	if err != nil {
		log.Error("Cannot compact index", zap.Error(err))
		return err
	}

	if err = f.Sync(); err != nil {
		log.Error("Cannot sync index file", zap.Error(err))
		return err
	}

	// Close file.
	if err := f.Close(); err != nil {
		log.Error("Cannot close index file", zap.Error(err))
		return err
	}

	// todo(vinland):// Reopen as an index file.

	elapsed := time.Since(start)
	log.Info("index compacted",
		logger.DurationLiteral("elapsed", elapsed),
		zap.Int64("bytes", n),
		zap.Int("kb_per_sec", int(float64(n)/elapsed.Seconds())/1024),
	)

	// fmt.Printf("write %d byte to file\n", n)

	return nil
}

// CompactTo compacts the in-memory index and writes it to w.
func (i *Index) CompactTo(w io.Writer) (n int64, err error) {
	// Wrap in bufferred writer with a buffer equivalent to the LogFile size.
	bw := bufio.NewWriterSize(w, indexFileBufferSize) // 128K

	// Setup compaction offset tracking data.
	var t IndexFileTrailer
	info := NewIndexFileCompactInfo()
	// info.cancel = cancel

	// Write magic number.
	if err := writeTo(bw, []byte(FileSignature), &n); err != nil {
		return n, err
	}

	// Retreve measurement names in order.
	names := i.MeasurementNames()

	// Flush buffer & mmap series block.
	// todo(vinland): series block?
	if err := bw.Flush(); err != nil {
		return n, err
	}

	// Write grid blocks in measurement order.
	if err := i.WriteGridBlockTo(bw, names, info, &n); err != nil {
		return n, err
	}

	// Write measurement block.
	t.MeasurementBlock.Offset = n
	if err := i.WriteMeasurementBlockTo(bw, names, info, &n); err != nil {
		return n, err
	}
	t.MeasurementBlock.Size = n - t.MeasurementBlock.Offset

	// Write trailer.
	nn, err := t.WriteTo(bw)
	n += nn
	if err != nil {
		return n, err
	}

	// Flush buffer.
	if err := bw.Flush(); err != nil {
		return n, err
	}

	return n, nil
}

func (i *Index) MeasurementNames() []string {
	a := make([]string, 0, len(i.measurements.measurementId))
	for name := range i.measurements.measurementId {
		a = append(a, name)
	}
	sort.Strings(a)
	return a
}

func (i *Index) WriteGridBlockTo(w io.Writer, names []string, info *IndexFileCompactInfo, n *int64) error {
	for _, name := range names {
		if err := i.writeGridsForMeasurementTo(w, name, info, n); err != nil {
			return err
		}
	}
	return nil
}

// writeGridsForMeasurementTo writes a single tagset to w and saves the tagset offset.
func (i *Index) writeGridsForMeasurementTo(w io.Writer, name string, info *IndexFileCompactInfo, n *int64) error {
	mm, err := i.measurements.MeasurementByName([]byte(name))
	if err != nil {
		return err
	}

	// // Check for cancellation.
	// select {
	// case <-info.cancel:
	// 	return ErrCompactionInterrupted
	// default:
	// }

	// Save tagset offset to measurement.
	offset := *n

	enc := NewGridBlockEncoder(w)
	gridInfos := make([]*GridCompactInfo, 0, len(mm.gIndex.grids))
	for _, grid := range mm.gIndex.grids {
		gridInfo := &GridCompactInfo{offset: offset + enc.n}
		err = enc.EncodeGrid(grid)
		if err != nil {
			return err
		}
		gridInfo.size = offset + enc.n - gridInfo.offset
		gridInfos = append(gridInfos, gridInfo)
	}

	// Flush tag block.
	// err := enc.Close()
	*n += enc.N()
	if err != nil {
		return err
	}

	// Save tagset offset to measurement.
	size := *n - offset

	info.Mms[name] = &IndexFileMeasurementCompactInfo{Offset: offset, Size: size, gridInfos: gridInfos, MeasurementID: mm.measurementID}

	return nil
}

func (i *Index) WriteMeasurementBlockTo(w io.Writer, names []string, info *IndexFileCompactInfo, n *int64) error {
	mw := NewMeasurementBlockWriter()

	// // Check for cancellation.
	// select {
	// case <-info.cancel:
	// 	return ErrCompactionInterrupted
	// default:
	// }

	// Add measurement data.
	for _, name := range names {
		mm, err := i.measurements.MeasurementByName([]byte(name))
		if err != nil {
			return ErrMeasurementNotFound
		}
		mmInfo := info.Mms[name]
		if mmInfo == nil {
			return ErrMeasurementNotFound
		}
		mw.Add([]byte(mm.name), mmInfo, mm.CacheSeriesIDSet())
	}

	// Flush data to writer.
	nn, err := mw.WriteTo(w)
	*n += nn
	return err
}
