package stdlib

// Code generated by 'goexports image'. DO NOT EDIT.

import (
	"image"
	"reflect"
)

func init() {
	Value["image"] = map[string]reflect.Value{
		"Black":                  reflect.ValueOf(image.Black),
		"Decode":                 reflect.ValueOf(image.Decode),
		"DecodeConfig":           reflect.ValueOf(image.DecodeConfig),
		"ErrFormat":              reflect.ValueOf(image.ErrFormat),
		"NewAlpha":               reflect.ValueOf(image.NewAlpha),
		"NewAlpha16":             reflect.ValueOf(image.NewAlpha16),
		"NewCMYK":                reflect.ValueOf(image.NewCMYK),
		"NewGray":                reflect.ValueOf(image.NewGray),
		"NewGray16":              reflect.ValueOf(image.NewGray16),
		"NewNRGBA":               reflect.ValueOf(image.NewNRGBA),
		"NewNRGBA64":             reflect.ValueOf(image.NewNRGBA64),
		"NewNYCbCrA":             reflect.ValueOf(image.NewNYCbCrA),
		"NewPaletted":            reflect.ValueOf(image.NewPaletted),
		"NewRGBA":                reflect.ValueOf(image.NewRGBA),
		"NewRGBA64":              reflect.ValueOf(image.NewRGBA64),
		"NewUniform":             reflect.ValueOf(image.NewUniform),
		"NewYCbCr":               reflect.ValueOf(image.NewYCbCr),
		"Opaque":                 reflect.ValueOf(image.Opaque),
		"Pt":                     reflect.ValueOf(image.Pt),
		"Rect":                   reflect.ValueOf(image.Rect),
		"RegisterFormat":         reflect.ValueOf(image.RegisterFormat),
		"Transparent":            reflect.ValueOf(image.Transparent),
		"White":                  reflect.ValueOf(image.White),
		"YCbCrSubsampleRatio410": reflect.ValueOf(image.YCbCrSubsampleRatio410),
		"YCbCrSubsampleRatio411": reflect.ValueOf(image.YCbCrSubsampleRatio411),
		"YCbCrSubsampleRatio420": reflect.ValueOf(image.YCbCrSubsampleRatio420),
		"YCbCrSubsampleRatio422": reflect.ValueOf(image.YCbCrSubsampleRatio422),
		"YCbCrSubsampleRatio440": reflect.ValueOf(image.YCbCrSubsampleRatio440),
		"YCbCrSubsampleRatio444": reflect.ValueOf(image.YCbCrSubsampleRatio444),
		"ZP":                     reflect.ValueOf(image.ZP),
		"ZR":                     reflect.ValueOf(image.ZR),
	}

	Type["image"] = map[string]reflect.Type{
		"Alpha":               reflect.TypeOf((*image.Alpha)(nil)).Elem(),
		"Alpha16":             reflect.TypeOf((*image.Alpha16)(nil)).Elem(),
		"CMYK":                reflect.TypeOf((*image.CMYK)(nil)).Elem(),
		"Config":              reflect.TypeOf((*image.Config)(nil)).Elem(),
		"Gray":                reflect.TypeOf((*image.Gray)(nil)).Elem(),
		"Gray16":              reflect.TypeOf((*image.Gray16)(nil)).Elem(),
		"Image":               reflect.TypeOf((*image.Image)(nil)).Elem(),
		"NRGBA":               reflect.TypeOf((*image.NRGBA)(nil)).Elem(),
		"NRGBA64":             reflect.TypeOf((*image.NRGBA64)(nil)).Elem(),
		"NYCbCrA":             reflect.TypeOf((*image.NYCbCrA)(nil)).Elem(),
		"Paletted":            reflect.TypeOf((*image.Paletted)(nil)).Elem(),
		"PalettedImage":       reflect.TypeOf((*image.PalettedImage)(nil)).Elem(),
		"Point":               reflect.TypeOf((*image.Point)(nil)).Elem(),
		"RGBA":                reflect.TypeOf((*image.RGBA)(nil)).Elem(),
		"RGBA64":              reflect.TypeOf((*image.RGBA64)(nil)).Elem(),
		"Rectangle":           reflect.TypeOf((*image.Rectangle)(nil)).Elem(),
		"Uniform":             reflect.TypeOf((*image.Uniform)(nil)).Elem(),
		"YCbCr":               reflect.TypeOf((*image.YCbCr)(nil)).Elem(),
		"YCbCrSubsampleRatio": reflect.TypeOf((*image.YCbCrSubsampleRatio)(nil)).Elem(),
	}
}
