package unixfs_errors

import (
	"testing"

	"github.com/pkg/errors"
)

func TestNewUnixFSError(t *testing.T) {
	tests := []struct {
		name          string
		inputError    error
		expectedError *UnixFSError
	}{
		{
			name:          "Nil error",
			inputError:    nil,
			expectedError: nil,
		},
		{
			name:       "Error FS not found",
			inputError: ErrFsNotFound,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_FS_NOT_FOUND,
				ErrorBody: "",
			},
		},
		{
			name:       "Error exists",
			inputError: ErrExist,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_EXIST,
				ErrorBody: "",
			},
		},
		{
			name:       "Error does not exist",
			inputError: ErrNotExist,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_NOT_EXIST,
				ErrorBody: "",
			},
		},
		{
			name:       "Error closed",
			inputError: ErrClosed,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_CLOSED,
				ErrorBody: "",
			},
		},
		{
			name:       "Error read only",
			inputError: ErrReadOnly,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_READ_ONLY,
				ErrorBody: "",
			},
		},
		{
			name:       "Error released",
			inputError: ErrReleased,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_RELEASED,
				ErrorBody: "",
			},
		},
		{
			name:       "Error not a directory",
			inputError: ErrNotDirectory,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_NOT_DIRECTORY,
				ErrorBody: "",
			},
		},
		{
			name:       "Error not a file",
			inputError: ErrNotFile,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_NOT_FILE,
				ErrorBody: "",
			},
		},
		{
			name:       "Error out of bounds",
			inputError: ErrOutOfBounds,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_OUT_OF_BOUNDS,
				ErrorBody: "",
			},
		},
		{
			name:       "Error empty path",
			inputError: ErrEmptyPath,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_EMPTY_PATH,
				ErrorBody: "",
			},
		},
		{
			name:       "Error inode unresolvable",
			inputError: ErrInodeUnresolvable,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_INODE_UNRESOLVABLE,
				ErrorBody: "",
			},
		},
		{
			name:       "Error not a symlink",
			inputError: ErrNotSymlink,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_NOT_SYMLINK,
				ErrorBody: "",
			},
		},
		{
			name:       "Error empty timestamp",
			inputError: ErrEmptyTimestamp,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_EMPTY_TIMESTAMP,
				ErrorBody: "",
			},
		},
		{
			name:       "Error move to self",
			inputError: ErrMoveToSelf,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_MOVE_TO_SELF,
				ErrorBody: "",
			},
		},
		{
			name:       "Error invalid write",
			inputError: ErrInvalidWrite,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_INVALID_WRITE,
				ErrorBody: "",
			},
		},
		{
			name:       "Error empty UnixFS ID",
			inputError: ErrEmptyUnixFsId,
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_EMPTY_UNIXFS_ID,
				ErrorBody: "",
			},
		},
		{
			name:       "Unknown Error",
			inputError: errors.New("unknown error"),
			expectedError: &UnixFSError{
				ErrorType: UnixFSErrorType_OTHER,
				ErrorBody: "unknown error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewUnixFSError(tt.inputError)
			if result == nil && tt.expectedError == nil {
				return // Correctly returned nil for nil input
			}
			if result == nil || tt.expectedError == nil {
				t.Fatalf("Expected error, got nil or vice versa")
			}
			if !result.EqualVT(tt.expectedError) {
				t.Errorf("Expected error %v, got %v", tt.expectedError, result)
			}
		})
	}
}
