package serializer

import (
	"time"
)

type Location string

func (l Location) ToSTD() (*time.Location, error) {
	return time.LoadLocation(string(l))
}
