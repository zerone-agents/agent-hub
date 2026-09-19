package personality

import "errors"

var (
	ErrNotFound      = errors.New("人格不存在")
	ErrAlreadyExists = errors.New("人格标识已存在")
	ErrBuiltinDelete = errors.New("系统人格不能删除，可以停用或基于它新建人格")
)
