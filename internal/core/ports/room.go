package ports

type RoomNameGenerator interface {
	Generate() string
	Release(name string)
}
