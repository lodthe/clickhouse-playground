package restapi

type TagStorage interface {
	GetAll(image string) ([]string, error)
	Exists(image string, tag string) (bool, error)
}
