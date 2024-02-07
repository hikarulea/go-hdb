package driver

import (
	"fmt"
	"slices"
	"sync"
	"time"
)

const (
	counterBytesRead = iota
	counterBytesWritten
	numCounter
)

const (
	gaugeConn = iota
	gaugeTx
	gaugeStmt
	numGauge
)

const (
	timeRead = iota
	timeWrite
	timeAuth
	numTime
)

const (
	sqlTimeQuery = iota
	sqlTimePrepare
	sqlTimeExec
	sqlTimeCall
	sqlTimeFetch
	sqlTimeFetchLob
	sqlTimeRollback
	sqlTimeCommit
	numSQLTime
)

type histogram struct {
	count          uint64
	sum            float64
	upperBounds    []float64
	boundCounts    []uint64
	underflowCount uint64 // in case of negative duration (will add to zero bucket)
}

func newHistogram(upperBounds []float64) *histogram {
	return &histogram{upperBounds: upperBounds, boundCounts: make([]uint64, len(upperBounds))}
}

func (h *histogram) stats() *StatsHistogram {
	rv := &StatsHistogram{
		Count:   h.count,
		Sum:     h.sum,
		Buckets: make(map[float64]uint64, len(h.upperBounds)),
	}
	for i, upperBound := range h.upperBounds {
		rv.Buckets[upperBound] = h.boundCounts[i]
	}
	return rv
}

func (h *histogram) add(v float64) {
	h.count++
	if v < 0 {
		h.underflowCount++
		v = 0
	}
	h.sum += v
	// determine index
	idx, _ := slices.BinarySearch(h.upperBounds, v)
	for i := idx; i < len(h.upperBounds); i++ {
		h.boundCounts[i]++
	}
}

type counterMsg struct {
	v   uint64
	idx int
}

type gaugeMsg struct {
	v   int64
	idx int
}

type timeMsg struct {
	d   time.Duration
	idx int
}

type sqlTimeMsg struct {
	d   time.Duration
	idx int
}

type metrics struct {
	mu sync.RWMutex

	parentMetrics *metrics

	timeUnit string
	divider  float64

	counters []uint64
	gauges   []int64
	times    []*histogram
	sqlTimes []*histogram
}

func newMetrics(parentMetrics *metrics, timeUnit string, timeUpperBounds []float64) *metrics {
	d, ok := timeUnitMap[timeUnit]
	if !ok {
		panic("invalid unit " + timeUnit)
	}
	rv := &metrics{
		parentMetrics: parentMetrics,
		timeUnit:      timeUnit,
		divider:       float64(d),
		counters:      make([]uint64, numCounter),
		gauges:        make([]int64, numGauge),
		times:         make([]*histogram, numTime),
		sqlTimes:      make([]*histogram, numSQLTime),
	}
	for i := 0; i < int(numTime); i++ {
		rv.times[i] = newHistogram(timeUpperBounds)
	}
	for i := 0; i < int(numSQLTime); i++ {
		rv.sqlTimes[i] = newHistogram(timeUpperBounds)
	}
	return rv
}

func (m *metrics) stats() *Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sqlTimes := make(map[string]*StatsHistogram, len(m.sqlTimes))
	for i, sqlTime := range m.sqlTimes {
		sqlTimes[statsCfg.SQLTimeTexts[i]] = sqlTime.stats()
	}
	return &Stats{
		OpenConnections:  int(m.gauges[gaugeConn]),
		OpenTransactions: int(m.gauges[gaugeTx]),
		OpenStatements:   int(m.gauges[gaugeStmt]),
		ReadBytes:        m.counters[counterBytesRead],
		WrittenBytes:     m.counters[counterBytesWritten],
		TimeUnit:         m.timeUnit,
		ReadTime:         m.times[timeRead].stats(),
		WriteTime:        m.times[timeWrite].stats(),
		AuthTime:         m.times[timeAuth].stats(),
		SQLTimes:         sqlTimes,
	}
}

func (m *metrics) handleMsg(msg any) {
	m.mu.Lock()
	switch msg := msg.(type) {
	case counterMsg:
		m.counters[msg.idx] += msg.v
	case gaugeMsg:
		m.gauges[msg.idx] += msg.v
	case timeMsg:
		m.times[msg.idx].add(float64(msg.d.Nanoseconds()) / m.divider)
	case sqlTimeMsg:
		m.sqlTimes[msg.idx].add(float64(msg.d.Nanoseconds()) / m.divider)
	default:
		panic(fmt.Sprintf("invalid metric message type %T", msg))
	}
	m.mu.Unlock()

	if m.parentMetrics != nil {
		m.parentMetrics.handleMsg(msg)
	}
}

const (
	numMetricCollectorCh = 25
)

type metricsCollector struct {
	wg    *sync.WaitGroup
	msgCh chan any
}

func newMetricsCollector(metrics *metrics) *metricsCollector {
	mc := &metricsCollector{
		wg:    new(sync.WaitGroup),
		msgCh: make(chan any, numMetricCollectorCh),
	}
	mc.wg.Add(1)
	go mc.collect(mc.wg, mc.msgCh, metrics)
	return mc
}

func (mc *metricsCollector) collect(wg *sync.WaitGroup, chMsg <-chan any, metrics *metrics) {
	defer wg.Done()
	for msg := range chMsg {
		metrics.handleMsg(msg)
	}
}

func (mc *metricsCollector) close() {
	close(mc.msgCh)
	mc.wg.Wait()
}
