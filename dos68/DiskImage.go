package dos68

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jonas-koeritz/floppymount"
)

const (
	sectorsPerTrack = 18
	bytesPerSector  = 128
)

const (
	filetypeText         = 0x01
	ftcs                 = 0x01
	filetypeBinary       = 0x02
	ftsq                 = 0x02
	filetypeRandomByte   = 0x04
	ftrb                 = 0x04
	fileTypeRandomRecord = 0x05
	ftrr                 = 0x05
)

const (
	FILE_STATUS_NOT_ACTIVE       = 0x00
	FANA                         = 0x00
	FILE_STATUS_SEQUENTIAL_READ  = 0x01
	FASR                         = 0x01
	FILE_STATUS_SEQUENTIAL_WRITE = 0x02
	FASW                         = 0x02
	FILE_STATUS_RANDOM_ACCESS    = 0x03
	FARA                         = 0x03
)

type DiskImage struct {
	fs.Inode
	data []byte
}

var _ = (fs.NodeOnAdder)((*DiskImage)(nil))

func (i *DiskImage) Tracks() int {
	return 35
}

func (i *DiskImage) GetSector(a floppymount.SectorAddress) ([]byte, error) {
	if a.Track >= i.Tracks() || a.Sector >= sectorsPerTrack || a.Track < 0 || a.Sector < 0 {
		return nil, floppymount.ErrInvalidAddress
	}

	offset := a.Track*sectorsPerTrack*bytesPerSector + a.Sector*bytesPerSector

	return i.data[offset : offset+bytesPerSector], nil
}

func (i *DiskImage) GetFiles() ([]floppymount.File, error) {
	fibs := make([]floppymount.File, 0)

	sector, err := i.GetSector(floppymount.SectorAddress{Track: 0, Sector: 1})
	if err != nil {
		return nil, err
	}

	var dib *directoryBlock

	// Read the initial directory block and parse it
	dib, err = parseDirectoryBlock(sector)
	if err != nil {
		return nil, err
	}
	fibs = append(fibs, dib.fileInformationBlocks...)

	if !dib.hasNext() {
		return fibs, nil
	}

	// read subsequent directory blocks
	for {
		sector, err = i.GetSector(dib.next)
		if err != nil {
			return nil, err
		}

		dib, err = parseDirectoryBlock(sector)
		if err != nil {
			return nil, err
		}
		fibs = append(fibs, dib.fileInformationBlocks...)

		if !dib.hasNext() {
			break
		}
	}

	return fibs, nil
}

func (image *DiskImage) GetFileContents(f floppymount.File) ([]byte, error) {
	data := make([]byte, 0)
	fib := f.(*fib)

	sector, err := image.GetSector(fib.begin)
	if err != nil {
		return data, err
	}

	for i := 0; i < fib.length; i++ {
		data = append(data, sector[4:]...)
		if sector[0] == 0x00 && sector[1] == 0x00 {
			break
		}

		sector, err = image.GetSector(floppymount.SectorAddress{Track: int(sector[0] & 0x7F), Sector: int(sector[1] & 0x3F)})
		if err != nil {
			return data, err
		}
	}

	// Trim trailing 0x00 characters to make this look cleaner
	if fib.fileType == filetypeText {
		data = []byte(strings.Trim(string(data), "\x00"))
	}

	return data, nil
}

func (i *DiskImage) OnAdd(ctx context.Context) {
	fcbs, err := i.GetFiles()
	if err != nil {
		return
	}

	p := &i.Inode

	for k, f := range fcbs {
		file := f.(*fib)
		file.image = i
		child := p.NewPersistentInode(ctx, file, fs.StableAttr{
			Ino: 1000 + uint64(k),
		})
		p.AddChild(f.Name(), child, true)
	}
}

func (i *DiskImage) FreeSectors() int {
	sector, _ := i.GetSector(floppymount.SectorAddress{Track: 0, Sector: 1})
	return int(sector[0x17])<<8 | int(sector[0x18])
}

func (i *DiskImage) String() string {
	listing := "FILE NAME   FTFS  BTBS  ETES  NSEC\n"
	files, _ := i.GetFiles()
	for _, f := range files {
		listing += fmt.Sprintf("%s\n", f)
	}
	listing += fmt.Sprintf("\n FREE SECTORS= %d\n", i.FreeSectors())
	return listing
}

type fib struct {
	fs.Inode

	filename   string
	extension  string
	fileType   uint8
	fileStatus uint8
	begin      floppymount.SectorAddress
	end        floppymount.SectorAddress
	length     int
	image      *DiskImage
}

var _ = (fs.FileReader)((*fib)(nil))
var _ = (fs.NodeOpener)((*fib)(nil))
var _ = (fs.NodeGetattrer)((*fib)(nil))

func (f *fib) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return f, fuse.FOPEN_DIRECT_IO, 0
}

func (f *fib) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = uint64(f.length) * (bytesPerSector - 4)
	return 0
}

func (f *fib) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	content, err := f.image.GetFileContents(f)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		return fuse.ReadResultData([]byte{}), syscall.EFAULT
	}

	end := off + int64(len(dest))
	if end > int64(len(content)) {
		end = int64(len(content))
	}

	return fuse.ReadResultData(content[off:end]), 0
}

func (fib *fib) Name() string {
	return fmt.Sprintf("%s.%s", fib.filename, fib.extension)
}

func (fib *fib) String() string {
	return fmt.Sprintf("%-6s.%-3s  %02X%02X  %02X%02X  %02X%02X  %4d",
		fib.filename, fib.extension,
		fib.fileType, fib.fileStatus,
		fib.begin.Track, fib.begin.Sector,
		fib.end.Track, fib.end.Sector,
		fib.length,
	)
}

func (fib *fib) Creation() time.Time {
	return time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
}

type directoryBlock struct {
	fileInformationBlocks []floppymount.File
	next                  floppymount.SectorAddress
}

func parseDirectoryBlock(data []byte) (*directoryBlock, error) {
	fibs := make([]floppymount.File, 0)

	i := 0
	// Check if this is marked as the first directory block
	// The first FIB doesn't contain file information in this case
	// and can be skipped
	if data[8] == 0xFF {
		i = 1
	}

	for ; i < 5; i++ {
		fibData := data[8+i*24 : 8+i*24+25]
		newFib := &fib{
			filename:   strings.Trim(string(fibData[0:6]), "\x00"),
			extension:  strings.Trim(string(fibData[6:9]), "\x00"),
			fileType:   fibData[9],
			fileStatus: fibData[10],
			begin:      floppymount.SectorAddress{Track: int(fibData[11] & 0x7F), Sector: int(fibData[12] & 0x3F)},
			end:        floppymount.SectorAddress{Track: int(fibData[13] & 0x7F), Sector: int(fibData[14] & 0x3F)},
			length:     int(fibData[15])<<8 | int(fibData[16]),
		}
		if newFib.fileType > 0 {
			fibs = append(fibs, newFib)
		}
	}

	next := floppymount.SectorAddress{Track: int(data[0] & 0x7F), Sector: int(data[1] & 0x3F)}

	dib := &directoryBlock{
		fileInformationBlocks: fibs,
		next:                  next,
	}
	return dib, nil
}

func (d *directoryBlock) hasNext() bool {
	return !(d.next.Track == 0 && d.next.Sector == 0)
}

func OpenImage(path string) (*DiskImage, error) {
	imageFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	i := new(DiskImage)
	i.data, err = io.ReadAll(imageFile)
	if err != nil {
		return nil, err
	}

	return i, nil
}
