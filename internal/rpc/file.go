package rpc

import (
	"fmt"

	fileapp "github.com/tinoosan/agen8-mcp-server/internal/services/file/app"
	filerpc "github.com/tinoosan/agen8-mcp-server/internal/services/file/rpc"
)

const (
	MethodFilesListDir    = "files.listDir"
	MethodFilesGet        = "files.get"
	MethodFilesCreateDir  = "files.createDir"
	MethodFilesCreateFile = "files.createFile"
	MethodFilesMove       = "files.move"
	MethodFilesCopy       = "files.copy"
	MethodFilesDelete     = "files.delete"
	MethodFilesUpload     = "files.upload"
)

func RegisterFile(reg *Registry, fileSvc *fileapp.Service) error {
	if fileSvc == nil {
		return fmt.Errorf("file service is required")
	}
	handler := filerpc.MustNewHandler(fileSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodFilesListDir, false, handler.ListDir)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesGet, false, handler.Get)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesCreateDir, false, handler.CreateDir)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesCreateFile, false, handler.CreateFile)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesMove, false, handler.Move)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesCopy, false, handler.Copy)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesDelete, false, handler.Delete)
		},
		func() error {
			return AddBoundHandler(reg, MethodFilesUpload, false, handler.Upload)
		},
	)
}
