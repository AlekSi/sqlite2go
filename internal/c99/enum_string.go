// Code generated by "stringer -output enum_string.go -type=TypeKind,cond enum.go type.go"; DO NOT EDIT.

package c99

import "fmt"

const _TypeKind_name = "BoolCharIntLongLongLongSCharShortUCharUIntULongULongLongUShortFloatDoubleLongDoubleFloatComplexDoubleComplexLongDoubleComplexmaxTypeKind"

var _TypeKind_index = [...]uint8{0, 4, 8, 11, 15, 23, 28, 33, 38, 42, 47, 56, 62, 67, 73, 83, 95, 108, 125, 136}

func (i TypeKind) String() string {
	i -= 1
	if i < 0 || i >= TypeKind(len(_TypeKind_index)-1) {
		return fmt.Sprintf("TypeKind(%d)", i+1)
	}
	return _TypeKind_name[_TypeKind_index[i]:_TypeKind_index[i+1]]
}

const _cond_name = "condZerocondIfOffcondIfOncondIfSkipmaxCond"

var _cond_index = [...]uint8{0, 8, 17, 25, 35, 42}

func (i cond) String() string {
	if i < 0 || i >= cond(len(_cond_index)-1) {
		return fmt.Sprintf("cond(%d)", i)
	}
	return _cond_name[_cond_index[i]:_cond_index[i+1]]
}
