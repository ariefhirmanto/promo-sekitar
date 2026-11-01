package outlet

import (
	"errors"
	"strconv"
	"strings"
)

type InputRefresh struct {
	BBox string // south,west,north,east
}

func (input InputRefresh) ValidateInput() error {
	parts := strings.Split(input.BBox, ",")
	if len(parts) != 4 {
		return errors.New("bbox must be south,west,north,east")
	}

	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return errors.New("bbox has a value that isn't a number")
		}
		v[i] = f
	}

	if v[0] >= v[2] || v[1] >= v[3] {
		return errors.New("bbox south/west must be smaller than north/east")
	}

	return nil
}
