//go:generate daggen -type=ConstOp -output=math_const_gen.go
//go:generate daggen -type=AddOp -output=math_add_gen.go
//go:generate daggen -type=SubOp -output=math_sub_gen.go
//go:generate daggen -type=DivOp -output=math_div_gen.go
//go:generate daggen -type=PackMathOperandsOp -output=math_pack_gen.go
package library
