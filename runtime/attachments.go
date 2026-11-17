package runtime

import (
	"fmt"
	"io"
	"strings"

	"github.com/kasra-sh/zendawn-go/attachment"
	"github.com/kasra-sh/zendawn-go/session"
)

// Upload stores a new session attachment from r.
func (r *Runtime) Upload(sessionID, filename, mimeType string, reader io.Reader) (attachment.Meta, error) {
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return attachment.Meta{}, err
	}
	m, err := store.Put(filename, mimeType, attachment.SourceUpload, reader)
	if err != nil {
		return attachment.Meta{}, NewError(CodeInvalidRequest, err.Error(), false)
	}
	r.emit(NewEvent(EventAttachmentUploaded, sessionID, map[string]any{
		"id": m.ID, "filename": m.Filename, "mimeType": m.MimeType, "size": m.Size, "source": m.Source,
	}))
	return m, nil
}

// Download opens a session attachment for reading. Caller closes the reader.
func (r *Runtime) Download(sessionID, id string) (attachment.Meta, io.ReadCloser, error) {
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return attachment.Meta{}, nil, err
	}
	m, rc, err := store.Open(id)
	if err != nil {
		return attachment.Meta{}, nil, NewError(CodeInvalidRequest, err.Error(), false)
	}
	return m, rc, nil
}

// ListAttachments returns metadata for all session attachments.
func (r *Runtime) ListAttachments(sessionID string) ([]attachment.Meta, error) {
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return nil, err
	}
	return store.List(), nil
}

// RegisterArtifact copies a workdir file into the attachment store for host download.
func (r *Runtime) RegisterArtifact(sessionID, workdirPath string) (attachment.Meta, error) {
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return attachment.Meta{}, err
	}
	m, err := store.PutFile(workdirPath, attachment.SourceArtifact)
	if err != nil {
		return attachment.Meta{}, NewError(CodeInvalidRequest, err.Error(), false)
	}
	r.emit(NewEvent(EventAttachmentRegistered, sessionID, map[string]any{
		"id": m.ID, "filename": m.Filename, "mimeType": m.MimeType, "size": m.Size, "source": m.Source,
	}))
	return m, nil
}

func (r *Runtime) attachmentStore(sessionID string) (*attachment.Store, error) {
	s, ok := r.GetSession(sessionID)
	if !ok {
		return nil, NewError(CodeInternal, "session not found", false)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.attachments == nil {
		r.attachments = map[string]*attachment.Store{}
	}
	store, ok := r.attachments[sessionID]
	if !ok {
		wd := s.WorkDir
		if wd == "" {
			wd = r.WorkDir
		}
		store = attachment.NewStore(wd)
		r.attachments[sessionID] = store
	}
	return store, nil
}

func (r *Runtime) resolveAttachment(sessionID, id string) (filename, mimeType, relPath string, data []byte, err error) {
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return "", "", "", nil, err
	}
	m, data, err := store.ReadBytes(id)
	if err != nil {
		return "", "", "", nil, err
	}
	return m.Filename, m.MimeType, m.RelPath, data, nil
}

func (r *Runtime) userMessageParts(sessionID string, in session.UserInput) ([]session.ContentPart, error) {
	var parts []session.ContentPart
	if strings.TrimSpace(in.Text) != "" {
		parts = append(parts, session.ContentPart{Type: session.PartText, Text: in.Text})
	}
	if len(in.AttachmentIDs) == 0 {
		if len(parts) == 0 {
			return nil, NewError(CodeInvalidRequest, "empty input", false)
		}
		return parts, nil
	}
	store, err := r.attachmentStore(sessionID)
	if err != nil {
		return nil, err
	}
	for _, id := range in.AttachmentIDs {
		m, ok := store.Get(id)
		if !ok {
			return nil, NewError(CodeInvalidRequest, fmt.Sprintf("unknown attachment: %s", id), false)
		}
		if attachment.IsImageMime(m.MimeType) {
			parts = append(parts, session.ContentPart{
				Type:         session.PartImage,
				AttachmentID: m.ID,
				Filename:     m.Filename,
				MimeType:     m.MimeType,
			})
		} else {
			parts = append(parts, session.ContentPart{
				Type:         session.PartFile,
				AttachmentID: m.ID,
				Filename:     m.Filename,
				MimeType:     m.MimeType,
			})
		}
	}
	if len(parts) == 0 {
		return nil, NewError(CodeInvalidRequest, "empty input", false)
	}
	return parts, nil
}

func transcriptHasImages(items []session.Item) bool {
	for _, it := range items {
		if it.Type != session.ItemMessage {
			continue
		}
		for _, p := range it.Content {
			if p.Type == session.PartImage {
				return true
			}
		}
	}
	return false
}
