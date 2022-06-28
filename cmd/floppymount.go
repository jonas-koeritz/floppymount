package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/jonas-koeritz/floppymount"
	"github.com/jonas-koeritz/floppymount/dos68"
	"github.com/jonas-koeritz/floppymount/flex2"
)

func main() {
	imageType := flag.String("type", "flex2", "the type of the image file contents (dos68|flex2)")
	debug := flag.Bool("debug", false, "print FUSE debug information")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Printf("Usage:\n  floppymount [options] <image file path> <mount point>\n\nOptions:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	imagePath := flag.Arg(0)
	mountPoint := flag.Arg(1)

	var image floppymount.DiskImage
	var err error

	switch strings.ToLower(*imageType) {
	case "dos68":
		image, err = dos68.OpenImage(imagePath)
	case "flex2":
		image, err = flex2.OpenImage(imagePath)
	default:
		err = fmt.Errorf("unknown image type \"%s\"", *imageType)
	}

	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", image.String())

	opts := &fs.Options{}
	opts.Debug = *debug

	server, err := fs.Mount(mountPoint, image, opts)

	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		return
	}

	server.Wait()
}
