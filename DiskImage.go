package floppymount

import (
	"errors"

	"github.com/hanwen/go-fuse/v2/fs"
)

type DiskImage interface {
	fs.InodeEmbedder
	String() string
}

type File interface {
	fs.InodeEmbedder
	Name() string
}

type SectorAddress struct {
	Track  int
	Sector int
}

var ErrInvalidAddress = errors.New("invalid sector address")
