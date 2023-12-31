package tsi2

import (
	"fmt"

	"github.com/influxdata/influxdb/v2/models"

	"cycledb/pkg/tsdb"
)

type Measurement struct {
	measurementID uint64
	name          string
	gIndex        *GridIndex

	// fileSet: index files' names
	indexFiles []IndexFile
}

func NewMeasurement(i *GridIndex, name string, id uint64) *Measurement {
	return &Measurement{
		gIndex:        i,
		name:          name,
		measurementID: id,
	}
}

func (m *Measurement) CacheSeriesIDSet() *tsdb.SeriesIDSet {
	idsSet := m.gIndex.SeriesIDSet()
	resSet := tsdb.NewSeriesIDSet()
	idsSet.ForEach(func(id uint64) {
		resSet.Add(id)
	})
	return resSet
}

func (m *Measurement) SeriesIDSet() *tsdb.SeriesIDSet {
	idsSet := m.gIndex.SeriesIDSet()
	resSet := tsdb.NewSeriesIDSet()
	idsSet.ForEach(func(id uint64) {
		resSet.Add(m.FormatIdWithMeasurementID(id))
	})
	for _, indexFile := range m.indexFiles {
		resSet.MergeInPlace(indexFile.SeriesIDSet([]byte(m.name)))
	}
	return resSet
}

func (m *Measurement) SeriesIDSetForTagKey(key []byte) *tsdb.SeriesIDSet {
	idsSet := m.gIndex.SeriesIDSetForTagKey(string(key))
	resSet := tsdb.NewSeriesIDSet()
	idsSet.ForEach(func(id uint64) {
		resSet.Add(m.FormatIdWithMeasurementID(id))
	})
	for _, indexFile := range m.indexFiles {
		resSet.MergeInPlace(indexFile.SeriesIDSetForTagKey([]byte(m.name), key))
	}
	return resSet
}

func (m *Measurement) SeriesIDSetForTagValue(key, value []byte) *tsdb.SeriesIDSet {
	idsSet := m.gIndex.SeriesIDSetForTagValue(string(key), string(value))
	resSet := tsdb.NewSeriesIDSet()
	idsSet.ForEach(func(id uint64) {
		// idsSet.Remove(id)
		resSet.Add(m.FormatIdWithMeasurementID(id))
	})
	for _, indexFile := range m.indexFiles {
		resSet.MergeInPlace(indexFile.SeriesIDSetForTagValue([]byte(m.name), key, value))
	}
	// fmt.Printf("Measurement.SeriesIDSetForTagValue: resSet: %v\n", resSet)
	return resSet
}

func (m *Measurement) SetTags(tags models.Tags) (uint64, bool) {
	id, success := m.gIndex.SetTags(tags)
	id = m.FormatIdWithMeasurementID(id)
	if !success {
		// fmt.Printf("set tag pair set fails: tags: %v\n", tags)
		return id, false
	}
	// fmt.Printf("m.SetTags: id: %+v\n", id)
	return id, true
}

func (m *Measurement) FormatIdWithMeasurementID(indexId uint64) uint64 {
	return (m.measurementID << 24) | (indexId)
}

// one measurement map to one grid index
// 2-byte to address measurement, then 4-byte to address id in gIndex within, combined as series id
type Measurements struct {
	// no contribution to id, since the seriesid conversion happens in measurement
	measurementId map[string]uint64
	measurements  []*Measurement
}

func NewMeasurements() *Measurements {
	return &Measurements{
		measurementId: map[string]uint64{},
		measurements:  []*Measurement{},
	}
}

func (ms *Measurements) MeasurementByName(name []byte) (*Measurement, error) {
	id, exist := ms.measurementId[string(name)]
	if !exist {
		return nil, nil
	}
	if len(ms.measurements) <= int(id) {
		return nil, fmt.Errorf("inconsistent between measurementId and measurements")
	}
	return ms.measurements[id], nil
}

func (ms *Measurements) DropMeasurement(name []byte) error {
	if id, ok := ms.measurementId[string(name)]; ok {
		delete(ms.measurementId, string(name))
		ms.measurements[id] = nil
	}
	return nil
}

func (ms *Measurements) AppendMeasurement(name []byte) error {
	measurementId := uint64(len(ms.measurements))
	m := NewMeasurement(NewGridIndex(NewMultiplierOptimizer(10, 2)), string(name), measurementId)
	ms.measurementId[string(name)] = measurementId
	ms.measurements = append(ms.measurements, m)
	return nil
}

func (ms *Measurements) SetTags(name []byte, tags models.Tags) (uint64, bool) {
	m, err := ms.MeasurementByName(name)
	if err != nil || m == nil {
		return 0, false
	}
	return m.SetTags(tags)
}

func (ms *Measurements) HasTagKey(name, key []byte) (bool, error) {
	m, err := ms.MeasurementByName(name)
	if err != nil || m == nil {
		return false, err
	}
	return m.gIndex.HasTagKey(string(key)), nil
}

func (ms *Measurements) HasTagValue(name, key, value []byte) (bool, error) {
	m, err := ms.MeasurementByName(name)
	if err != nil || m == nil {
		return false, err
	}
	return m.gIndex.HasTagValue(string(key), string(value)), nil
}

func (ms *Measurements) MeasurementSeriesIDIterator(name []byte) (tsdb.SeriesIDIterator, error) {
	m, err := ms.MeasurementByName(name)
	if err != nil || m == nil {
		return nil, err
	}
	return NewSeriesIDSetIterator(m.SeriesIDSet()), nil
}

func (ms *Measurements) TagKeySeriesIDIterator(name, key []byte) (tsdb.SeriesIDSetIterator, error) {
	m, err := ms.MeasurementByName(name)
	if err != nil || m == nil {
		return nil, err
	}
	return NewSeriesIDSetIterator(m.SeriesIDSetForTagKey(key)), nil
}

func (ms *Measurements) TagValueSeriesIDIterator(name, key, value []byte) (tsdb.SeriesIDSetIterator, error) {
	m, err := ms.MeasurementByName(name)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return NewSeriesIDSetIterator(tsdb.NewSeriesIDSet()), nil
	}
	return NewSeriesIDSetIterator(m.SeriesIDSetForTagValue(key, value)), nil
}
