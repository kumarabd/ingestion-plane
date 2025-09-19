package service

type DataLayer interface {
	Ping() (bool, error)
}
