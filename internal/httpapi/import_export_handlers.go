package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"video-record/internal/records"
)

const importFileLimit = 10 << 20

func (handlers recordHandlers) exportData(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	format := records.ExportFormat(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))))
	file, err := handlers.service.ExportData(r.Context(), identity.User.ID, format)
	if err != nil {
		if errors.Is(err, records.ErrInvalidExport) {
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_export")
			return
		}
		writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		return
	}
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func (handlers recordHandlers) importData(w http.ResponseWriter, r *http.Request) {
	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeProblem(w, r, http.StatusUnauthorized, "Unauthorized", "unauthenticated")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
		return
	}
	var filename string
	var data []byte
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
			return
		}
		if part.FormName() != "file" || part.FileName() == "" || filename != "" {
			_ = part.Close()
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
			return
		}
		filename = part.FileName()
		data, err = io.ReadAll(io.LimitReader(part, importFileLimit+1))
		_ = part.Close()
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
			return
		}
	}
	if filename == "" {
		writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
		return
	}
	report, err := handlers.service.ImportData(r.Context(), identity.User.ID, filename, data)
	if err != nil {
		switch {
		case errors.Is(err, records.ErrImportTooLarge):
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "Content Too Large", "import_too_large")
		case errors.Is(err, records.ErrInvalidImport), errors.Is(err, records.ErrUnsafeImportFilename):
			writeProblem(w, r, http.StatusBadRequest, "Bad Request", "invalid_import")
		default:
			writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "internal_error")
		}
		return
	}
	writeJSON(w, http.StatusOK, report)
}
