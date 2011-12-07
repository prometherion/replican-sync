package fs

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"path/filepath"
)

// Block size used for checksum, comparison, transmitting deltas.
const BLOCKSIZE int = 8192

// Nodes are any member of a hierarchical tree model representing 
// a part of the filesystem. Nodes include files and directories,
// and also blocks within the files.
type Node interface {
	Repo() NodeRepo

	// Get the node that contains this node in the hierarchical index.
	Parent() (FsNode, bool)
}

// FsNodes are members of a hierarchical index that map directly onto the filesystem:
// files and directories.
type FsNode interface {

	// FsNode extends the concept of Node.
	Node

	// All FsNodes have names (file or directory name).
	Name() string
}

// Given a filesystem node, calculate the relative path string to it from the root node.
func RelPath(item FsNode) string {
	parts := []string{}

	for fsNode, hasParent := item, true; hasParent; 
			fsNode, hasParent = fsNode.Parent() {
		parts = append([]string{fsNode.Name()}, parts...)
	}

	return filepath.Join(parts...)
}

type Block interface {
	Node

	Info() *BlockInfo
}

// Represent a block in a hierarchical tree model.
// Blocks are BLOCKSIZE chunks of data which comprise files.
type BlockInfo struct {
	Position int
	Weak     int
	Strong   string
	Parent   string
}

// Get the byte offset of this block in its containing file.
func (block *BlockInfo) Offset() int64 {
	return int64(block.Position) * int64(BLOCKSIZE)
}

type File interface {
	FsNode

	Info() *FileInfo

	Blocks() []Block
}

// Represent a file in a hierarchical tree model.
type FileInfo struct {
	Name   string
	Mode   uint32
	Size   int64
	Strong string
	Parent string
}

type Dir interface {
	FsNode

	Info() *DirInfo

	SubDirs() []Dir

	Files() []File
}

// Represent a directory in a hierarchical tree model.
type DirInfo struct {
	Name   string
	Mode   uint32
	Strong string
	Parent string
}

// Calculate the strong checksum of a directory.
func DirStrong(dir Dir) string {
	repo := dir.Repo()
	oldStrong := dir.Info().Strong
	var sha1 = sha1.New()
	sha1.Write(DirContents(dir))
	newStrong := toHexString(sha1)
	
	if oldStrong != newStrong {
		for _, file := range repo.Files(oldStrong) {
			fileInfo := file.Info()
			fileInfo.Parent = newStrong
			repo.SetFile(fileInfo)
		}
		for _, subdir := range repo.SubDirs(oldStrong) {
			subdirInfo := subdir.Info()
			subdirInfo.Parent = newStrong
			repo.SetDir(subdirInfo)
		}
		dir.Info().Strong = newStrong
		repo.SetDir(dir.Info())
		repo.Remove(oldStrong)
	}
	
	return newStrong
}

// Represent the directory's distinct deep contents as a byte array.
// Inspired by skimming over git internals.
func DirContents(dir Dir) []byte {
	buf := bytes.NewBufferString("")

	for _, subdir := range dir.SubDirs() {
		fmt.Fprintf(buf, "%s\td\t%s\n", DirStrong(subdir), subdir.Name())
	}
	for _, file := range dir.Files() {
		fmt.Fprintf(buf, "%s\tf\t%s\n", file.Info().Strong, file.Name())
	}

	return buf.Bytes()
}

func DirLookup(dir Dir, relpath string) (fsNode FsNode, hasItem bool) {
	parts := SplitNames(relpath)
	cwd := dir
	i := 0
	l := len(parts)

	for i = 0; i < l; i++ {
		fsNode, hasItem = DirItem(cwd, parts[i])
		if !hasItem {
			return nil, false
		}

		if i == l-1 {
			return fsNode, true
		}

		switch t := fsNode.(type) {
		case Dir:
			cwd = fsNode.(Dir)
		default:
			return nil, false
		}
	}

	return nil, false
}

func DirItem(dir Dir, name string) (FsNode, bool) {
	for _, subdir := range dir.SubDirs() {
		if subdir.Name() == name {
			return subdir, true
		}
	}

	for _, file := range dir.Files() {
		if file.Name() == name {
			return file, true
		}
	}

	return nil, false
}

// Visitor function to traverse a hierarchical tree model.
type NodeVisitor func(Node) bool

// Traverse the hierarchical tree model with a user-defined NodeVisitor function.
func Walk(node Node, visitor NodeVisitor) {
	nodestack := []Node{}
	nodestack = append(nodestack, node)

	for len(nodestack) > 0 {
		current := nodestack[0]
		nodestack = nodestack[1:]
		if visitor(current) {

			if dir, isDir := current.(Dir); isDir {
				for _, subdir := range dir.SubDirs() {
					nodestack = append(nodestack, subdir)
				}
				for _, file := range dir.Files() {
					nodestack = append(nodestack, file)
				}
			} else if file, isFile := current.(File); isFile {
				for _, block := range file.Blocks() {
					nodestack = append(nodestack, block)
				}
			}

		}
	}
}
