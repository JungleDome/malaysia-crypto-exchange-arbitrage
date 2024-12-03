package domain

type SlippageDetectionModeEnum int

const (
	Price SlippageDetectionModeEnum = iota
	Percentage
)

func (e SlippageDetectionModeEnum) String() string {
	return []string{"Price", "Percentage"}[e]
}

type ArbitrageWatcherModeEnum int

const (
	Scheduled ArbitrageWatcherModeEnum = iota
	Stream
)

func (e ArbitrageWatcherModeEnum) String() string {
	return []string{"Scheduled", "Stream"}[e]
}
