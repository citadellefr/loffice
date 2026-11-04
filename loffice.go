// Package loffice serves Word, Excel and PowerPoint documents, CSV and plain
// text files to collaborative editors. The hub is trame's: this package is
// the format that reads those files into documents, checks their edits,
// calculates their formulas and writes them back.
package loffice

import (
	"context"
	"errors"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame"
)

type (
	Conn    = trame.Conn
	Store   = trame.Store
	Peer    = trame.Peer
	Options = trame.Options
)

// Close codes sent to a client whose connection the hub ends.
const (
	CloseLoadFailed = trame.CloseLoadFailed
	CloseRevoked    = trame.CloseRevoked
	CloseTooSlow    = trame.CloseTooSlow
	CloseShutdown   = trame.CloseShutdown
)

var (
	ErrClosed = trame.ErrClosed
	// ErrGone is returned (possibly wrapped) by Store.Save when the file no
	// longer exists: see trame.ErrGone.
	ErrGone    = trame.ErrGone
	ErrNoMedia = partrel.ErrNoMedia
	// ErrTooLarge is a file the hub refuses for its size, or a package
	// that holds more than its Limits allow.
	ErrTooLarge = opc.ErrTooLarge
	// ErrPicture is a picture a document does not take: not a PNG, JPEG or
	// GIF file, too large, or a format without pictures.
	ErrPicture = errors.New("loffice: this picture cannot be added")
)

// Hub serves any number of documents. Each one is loaded when its first peer
// connects and leaves memory once the last one is gone and it is saved. The
// key a document is served under ends with the file's extension, which
// picks its format.
type Hub struct {
	*trame.Hub
}

// NewHub makes a hub that keeps its files in store. limits bound the files
// it opens: see Limits.
func NewHub(store Store, opt Options, limits Limits) *Hub {
	return &Hub{trame.NewHub(store, limits.withDefaults().open, opt)}
}

// Media is a picture of a document, by the name its nodes give it, and its
// content type. The document is read if nobody has it open.
func (h *Hub) Media(ctx context.Context, key, name string) (data []byte, typ string, err error) {
	err = h.Use(ctx, key, func(f trame.File) error {
		m, ok := f.(mediaFile)
		if !ok {
			return ErrNoMedia
		}
		data, typ, err = m.media(name)
		return err
	})
	return data, typ, err
}

// AddPicture keeps a picture for the drawings of a document, and gives the
// name a client shows it by. Pictures never travel through the socket.
func (h *Hub) AddPicture(ctx context.Context, key string, data []byte) (name string, err error) {
	err = h.Use(ctx, key, func(f trame.File) error {
		adder, ok := f.(pictureAdder)
		if !ok {
			return ErrPicture
		}
		name, err = adder.addPicture(data)
		return err
	})
	return name, err
}
