package flex2

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/jonas-koeritz/floppymount"
)

const (
	sectorsPerTrack = 10
	bytesPerSector  = 256
)

type DiskImage struct {
	fs.Inode
	file      *os.File
	directory *directory
}

type directory []*fcb

func (image *DiskImage) getDirectory() *directory {
	if image.directory == nil {
		image.readDirectory()
	}
	return image.directory
}

func (image *DiskImage) readDirectory() {
	dir := make(directory, 0)

	// read the chain of sectors that make up the directory
	directoryData := image.getSectors(floppymount.SectorAddress{Track: 0, Sector: 5}, 16)

	for i := 0; i < len(directoryData)/24; i++ {
		fcbData := directoryData[i*24 : i*24+24]

		dir = append(dir, &fcb{
			index:              i,
			filename:           strings.Trim(string(fcbData[0:8]), "\x00"),
			extension:          strings.Trim(string(fcbData[8:11]), "\x00"),
			attributes:         fcbData[11],
			begin:              floppymount.SectorAddress{Track: int(fcbData[13]), Sector: int(fcbData[14])},
			end:                floppymount.SectorAddress{Track: int(fcbData[15]), Sector: int(fcbData[16])},
			length:             int(fcbData[17])<<8 | int(fcbData[18]),
			sectorMapIndicator: fcbData[19],
			creation:           time.Date(int(fcbData[23])+1900, time.Month(fcbData[21]), int(fcbData[22]), 0, 0, 0, 0, time.Local),
			deleted:            fcbData[0]&0x80 == 0x80,
			allocated:          fcbData[13] != 0 || fcbData[14] != 0,
			image:              image,
		})
	}
	image.directory = &dir
}

var _ = (fs.NodeOnAdder)((*DiskImage)(nil))

func (i *DiskImage) tracks() int {
	sector, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return int(sector[0x26]) + 1
}

func (i *DiskImage) label() string {
	sir, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return strings.Trim(string(sir[0x10:0x18]), "\x00")
}

func (i *DiskImage) freeSectors() int {
	sir, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return int(sir[0x21])<<8 | int(sir[0x22])
}

func (i *DiskImage) creationDate() time.Time {
	sir, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return time.Date(int(sir[0x25])+1900, time.Month(sir[0x23]), int(sir[0x24]), 0, 0, 0, 0, time.Local)
}

func (i *DiskImage) firstFree() floppymount.SectorAddress {
	sir, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return floppymount.SectorAddress{Track: int(sir[0x1d]), Sector: int(sir[0x1e])}
}

func (i *DiskImage) lastFree() floppymount.SectorAddress {
	sir, _ := i.getSector(floppymount.SectorAddress{Track: 0, Sector: 3})
	return floppymount.SectorAddress{Track: int(sir[0x1f]), Sector: int(sir[0x20])}
}

func (i *DiskImage) getSectors(begin floppymount.SectorAddress, skip int) []byte {
	data := make([]byte, 0)
	sector, _ := i.getSector(begin)
	data = append(data, sector[skip:]...)
	for {
		data = append(data, sector[skip:]...)
		if sector[0] == 00 && sector[1] == 00 {
			break
		}

		sector, _ = i.getSector(floppymount.SectorAddress{Track: int(sector[0]), Sector: int(sector[1])})
	}
	return data
}

func (image *DiskImage) getFileContents(f floppymount.File) ([]byte, error) {
	fcb := f.(*fcb)
	data := image.getSectors(fcb.begin, 4)

	// Handle space compression for TXT files
	if strings.EqualFold(fcb.extension, "TXT") {
		uncompressed := make([]byte, 0)
		for i := 0; i < len(data); i++ {
			if data[i] == 0x09 {
				uncompressed = append(uncompressed, []byte(strings.Repeat(" ", int(data[i+1])))...)
				i++
			} else {
				uncompressed = append(uncompressed, data[i])
			}
		}
		// remove sector slack
		data = []byte(strings.Trim(string(uncompressed), "\x00"))
	}

	return data, nil
}

func (i *DiskImage) getSector(a floppymount.SectorAddress) ([]byte, error) {
	if a.Sector > sectorsPerTrack || a.Track < 0 || a.Sector < 1 {
		return nil, floppymount.ErrInvalidAddress
	}

	offset := a.Track*sectorsPerTrack*bytesPerSector + (a.Sector-1)*bytesPerSector

	sector := make([]byte, bytesPerSector)
	_, err := i.file.ReadAt(sector, int64(offset))

	return sector, err
}

func (i *DiskImage) OnAdd(ctx context.Context) {
	dir := i.getDirectory()

	p := &i.Inode

	for k, f := range *dir {
		if !f.deleted && f.allocated {
			child := p.NewPersistentInode(ctx, f, fs.StableAttr{
				Ino: 1000 + uint64(k),
			})
			p.AddChild(f.Name(), child, true)
		}
	}
}

// String creates a human readble directory listing of the image
func (i *DiskImage) String() string {
	dir := i.getDirectory()

	listing := fmt.Sprintf("DISK LABEL: %s\nCREATED:    %s\n", i.label(), i.creationDate().Format("2006-01-02"))
	listing += "\n FILE NAME     ATTR  STRT  END   NSEC  SMI  DATE\n"

	for _, f := range *dir {
		if f.deleted || !f.allocated {
			continue
		}
		listing += " " + f.String() + "\n"
	}

	listing += fmt.Sprintf("\n FREE SECTORS= %d\n", i.freeSectors())
	return listing
}

type fcb struct {
	fs.Inode

	index              int
	filename           string
	extension          string
	attributes         uint8
	begin              floppymount.SectorAddress
	end                floppymount.SectorAddress
	length             int
	sectorMapIndicator uint8
	creation           time.Time
	deleted            bool
	allocated          bool

	image *DiskImage
}

var _ = (fs.NodeReader)((*fcb)(nil))
var _ = (fs.NodeOpener)((*fcb)(nil))
var _ = (fs.NodeGetattrer)((*fcb)(nil))

func (f *fcb) Read(ctx context.Context, fh fs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	content, err := f.image.getFileContents(f)
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

func (f *fcb) Open(ctx context.Context, openFlags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return f, fuse.FOPEN_DIRECT_IO, 0
}

func (f *fcb) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Size = uint64(f.length) * (bytesPerSector - 4)
	out.Ctime = uint64(f.Creation().Unix())
	out.Mtime = uint64(f.Creation().Unix())
	return 0
}

/*
func (f *fcb) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	if size, ok := in.GetSize(); ok {
		// f.resize(int(size))
	}
	return 0
}
*/

func (f *fcb) Name() string {
	return fmt.Sprintf("%s.%s", f.filename, f.extension)
}

func (f *fcb) String() string {
	return fmt.Sprintf("%-8s.%-3s  %02X    %02X%02X  %02X%02X  %4d  %02X   %04d-%02d-%02d",
		f.filename, f.extension,
		f.attributes,
		f.begin.Track, f.begin.Sector,
		f.end.Track, f.end.Sector,
		f.length,
		f.sectorMapIndicator,
		f.creation.Year(),
		f.creation.Month(),
		f.creation.Day(),
	)
}

func (f *fcb) Creation() time.Time {
	return f.creation
}

func OpenImage(path string) (*DiskImage, error) {
	imageFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	i := &DiskImage{file: imageFile}
	return i, nil
}
