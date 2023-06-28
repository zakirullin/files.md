package sched

import (
	"testing"

	"zakirullin/stuffbot/pkg/text"

	"github.com/stretchr/testify/require"
)

func TestUcfirst(t *testing.T) {
	r := require.New(t)

	res := text.Ucfirst("abc")

	r.Equal("Abc", res)
}

func TestUcfirstRu(t *testing.T) {
	r := require.New(t)

	res := text.Ucfirst("абв")

	r.Equal("Абв", res)
}
