# What does this do?

This toolset implements some legacy floppy disk formats as FUSE filesystems to make them easier to work with on a modern system.

# What doesn't this do?

Currently only **reading** of files is supported, the capability to write/change files as well es creating disk images from scratch will be added in the future.

# Which formats are supported?

* DOS68: 35 tracks, 18 sectors per track, 128 bytes per sector
* FLEX2: 40 or 80 tracks, 10 sectors per track, 256 bytes per sector

# How do I use it?

## Mounting a DOS68 image

The utility will output a listing of the image and mount it using FUSE.

```
$ floppymount --type=dos68 SSBDOS68.img ./mountpoint

FILE NAME   FTFS  BTBS  ETES  NSEC
DFM68O.352  5200  0002  0006     5
DFM68O.353  5200  0007  0010    10
LIST  .$    5200  0011  0103     5
DOS68 .51C  0200  0104  0308    41
SET   .$    0200  030A  0310     7
VIEW  .$    0200  0311  0400     2
[...]
ASMBC .$    0200  0E0F  110A    50
BASICC.$    0200  110B  1603    83
SYSEQU.TXT  0100  1604  1903    54
DO    .$    0200  1904  1907     4
PTCH  .TXT  0100  1909  1909     1
SYSPRT.BIN  0200  190A  190A     1
TART  .BAK  0100  190B  190B     1
START .BAK  0100  190C  190C     1
START .UP   0100  190D  190D     1

 FREE SECTORS= 167

```

To unmount the image just eject it using your file explorer or `fusermount -u ./mountpoint`

## Mounting a FLEX2 image

The utility will output a listing of the image and mount it using FUSE.

```
$ floppymount --type=flex2 F2UTIL1.DSK ./mountpoint
DISK LABEL: UTILITY1
CREATED:    1979-03-18

FILE NAME     ATTR  STRT  END   NSEC  SMI  DATE
WORDS   .TXT  00    1001  0D03     8  00   1979-05-12
UP-LOW  .TXT  00    0F05  0F0A     6  00   1979-05-12
CONTIN  .TXT  00    0B09  0C01     3  00   1979-05-12
DUMP    .TXT  00    1B01  1E0A     9  00   1979-05-12
DUMP    .CMD  00    1D08  1D08     1  00   1979-05-12
COPY    .CMD  00    0203  0207     5  00   1979-03-18
CMPMEM  .TXT  00    190A  1A06     7  00   1979-05-12
[...]
UP-LOW  .CMD  00    1309  1309     1  00   1979-05-12
WORDS   .CMD  00    1002  1002     1  00   1979-05-12
HELPP   .TXT  00    1701  1007    14  00   1980-04-29
HELP    .CMD  00    0909  090A     2  00   1980-04-29

 FREE SECTORS= 21

```

To unmount the image just eject it using your file explorer or `fusermount -u ./mountpoint`