package domain

type ExchangeEnum int

const (
	Luno ExchangeEnum = iota
	Hata
)

func (e ExchangeEnum) String() string {
	return []string{"Luno", "Hata"}[e]
}
