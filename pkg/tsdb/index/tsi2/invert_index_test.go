package tsi2_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"cycledb/pkg/tsdb/index/tsi2"
)

func TestInvertIndex(t *testing.T) {
	index := tsi2.NewInvertIndex()
	manyTagPairs := GetManyTagPairs(2, 5)
	// do insert
	for i, tagPairs := range manyTagPairs {
		_, id := index.InitNewSeriesID(tagPairs)
		assert.Equal(t, int64(i), id)
	}
	// do query
	for i, tagPairs := range manyTagPairs {
		realIds := index.GetSeriesIDsWithTagPairs(tagPairs)
		assert.True(t, Contains(realIds, []int64{int64(i)}))
	}
}
