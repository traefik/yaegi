// +build go1.11,!go1.12

package syscall

// Code generated by 'goexports syscall'. DO NOT EDIT.

import (
	"reflect"
	"syscall"
)

func init() {
	Symbols["syscall"] = map[string]reflect.Value{
		// function, constant and variable definitions
		"Await":               reflect.ValueOf(syscall.Await),
		"Bind":                reflect.ValueOf(syscall.Bind),
		"BytePtrFromString":   reflect.ValueOf(syscall.BytePtrFromString),
		"ByteSliceFromString": reflect.ValueOf(syscall.ByteSliceFromString),
		"Chdir":               reflect.ValueOf(syscall.Chdir),
		"Clearenv":            reflect.ValueOf(syscall.Clearenv),
		"Close":               reflect.ValueOf(syscall.Close),
		"Create":              reflect.ValueOf(syscall.Create),
		"DMAPPEND":            reflect.ValueOf(syscall.DMAPPEND),
		"DMAUTH":              reflect.ValueOf(syscall.DMAUTH),
		"DMDIR":               reflect.ValueOf(uint32(syscall.DMDIR)),
		"DMEXCL":              reflect.ValueOf(syscall.DMEXCL),
		"DMEXEC":              reflect.ValueOf(syscall.DMEXEC),
		"DMMOUNT":             reflect.ValueOf(syscall.DMMOUNT),
		"DMREAD":              reflect.ValueOf(syscall.DMREAD),
		"DMTMP":               reflect.ValueOf(syscall.DMTMP),
		"DMWRITE":             reflect.ValueOf(syscall.DMWRITE),
		"Dup":                 reflect.ValueOf(syscall.Dup),
		"EACCES":              reflect.ValueOf(&syscall.EACCES).Elem(),
		"EAFNOSUPPORT":        reflect.ValueOf(&syscall.EAFNOSUPPORT).Elem(),
		"EBUSY":               reflect.ValueOf(&syscall.EBUSY).Elem(),
		"EEXIST":              reflect.ValueOf(&syscall.EEXIST).Elem(),
		"EINTR":               reflect.ValueOf(&syscall.EINTR).Elem(),
		"EINVAL":              reflect.ValueOf(&syscall.EINVAL).Elem(),
		"EIO":                 reflect.ValueOf(&syscall.EIO).Elem(),
		"EISDIR":              reflect.ValueOf(&syscall.EISDIR).Elem(),
		"EMFILE":              reflect.ValueOf(&syscall.EMFILE).Elem(),
		"ENAMETOOLONG":        reflect.ValueOf(&syscall.ENAMETOOLONG).Elem(),
		"ENOENT":              reflect.ValueOf(&syscall.ENOENT).Elem(),
		"ENOTDIR":             reflect.ValueOf(&syscall.ENOTDIR).Elem(),
		"EPERM":               reflect.ValueOf(&syscall.EPERM).Elem(),
		"EPLAN9":              reflect.ValueOf(&syscall.EPLAN9).Elem(),
		"ERRMAX":              reflect.ValueOf(syscall.ERRMAX),
		"ESPIPE":              reflect.ValueOf(&syscall.ESPIPE).Elem(),
		"ETIMEDOUT":           reflect.ValueOf(&syscall.ETIMEDOUT).Elem(),
		"Environ":             reflect.ValueOf(syscall.Environ),
		"ErrBadName":          reflect.ValueOf(&syscall.ErrBadName).Elem(),
		"ErrBadStat":          reflect.ValueOf(&syscall.ErrBadStat).Elem(),
		"ErrShortStat":        reflect.ValueOf(&syscall.ErrShortStat).Elem(),
		"Exec":                reflect.ValueOf(syscall.Exec),
		"Exit":                reflect.ValueOf(syscall.Exit),
		"Fchdir":              reflect.ValueOf(syscall.Fchdir),
		"Fd2path":             reflect.ValueOf(syscall.Fd2path),
		"Fixwd":               reflect.ValueOf(syscall.Fixwd),
		"ForkExec":            reflect.ValueOf(syscall.ForkExec),
		"ForkLock":            reflect.ValueOf(&syscall.ForkLock).Elem(),
		"Fstat":               reflect.ValueOf(syscall.Fstat),
		"Fwstat":              reflect.ValueOf(syscall.Fwstat),
		"Getegid":             reflect.ValueOf(syscall.Getegid),
		"Getenv":              reflect.ValueOf(syscall.Getenv),
		"Geteuid":             reflect.ValueOf(syscall.Geteuid),
		"Getgid":              reflect.ValueOf(syscall.Getgid),
		"Getgroups":           reflect.ValueOf(syscall.Getgroups),
		"Getpagesize":         reflect.ValueOf(syscall.Getpagesize),
		"Getpid":              reflect.ValueOf(syscall.Getpid),
		"Getppid":             reflect.ValueOf(syscall.Getppid),
		"Gettimeofday":        reflect.ValueOf(syscall.Gettimeofday),
		"Getuid":              reflect.ValueOf(syscall.Getuid),
		"Getwd":               reflect.ValueOf(syscall.Getwd),
		"ImplementsGetwd":     reflect.ValueOf(syscall.ImplementsGetwd),
		"MAFTER":              reflect.ValueOf(syscall.MAFTER),
		"MBEFORE":             reflect.ValueOf(syscall.MBEFORE),
		"MCACHE":              reflect.ValueOf(syscall.MCACHE),
		"MCREATE":             reflect.ValueOf(syscall.MCREATE),
		"MMASK":               reflect.ValueOf(syscall.MMASK),
		"MORDER":              reflect.ValueOf(syscall.MORDER),
		"MREPL":               reflect.ValueOf(syscall.MREPL),
		"Mkdir":               reflect.ValueOf(syscall.Mkdir),
		"Mount":               reflect.ValueOf(syscall.Mount),
		"NewError":            reflect.ValueOf(syscall.NewError),
		"NsecToTimeval":       reflect.ValueOf(syscall.NsecToTimeval),
		"O_APPEND":            reflect.ValueOf(syscall.O_APPEND),
		"O_ASYNC":             reflect.ValueOf(syscall.O_ASYNC),
		"O_CLOEXEC":           reflect.ValueOf(syscall.O_CLOEXEC),
		"O_CREAT":             reflect.ValueOf(syscall.O_CREAT),
		"O_EXCL":              reflect.ValueOf(syscall.O_EXCL),
		"O_NOCTTY":            reflect.ValueOf(syscall.O_NOCTTY),
		"O_NONBLOCK":          reflect.ValueOf(syscall.O_NONBLOCK),
		"O_RDONLY":            reflect.ValueOf(syscall.O_RDONLY),
		"O_RDWR":              reflect.ValueOf(syscall.O_RDWR),
		"O_SYNC":              reflect.ValueOf(syscall.O_SYNC),
		"O_TRUNC":             reflect.ValueOf(syscall.O_TRUNC),
		"O_WRONLY":            reflect.ValueOf(syscall.O_WRONLY),
		"Open":                reflect.ValueOf(syscall.Open),
		"Pipe":                reflect.ValueOf(syscall.Pipe),
		"Pread":               reflect.ValueOf(syscall.Pread),
		"Pwrite":              reflect.ValueOf(syscall.Pwrite),
		"QTAPPEND":            reflect.ValueOf(syscall.QTAPPEND),
		"QTAUTH":              reflect.ValueOf(syscall.QTAUTH),
		"QTDIR":               reflect.ValueOf(syscall.QTDIR),
		"QTEXCL":              reflect.ValueOf(syscall.QTEXCL),
		"QTFILE":              reflect.ValueOf(syscall.QTFILE),
		"QTMOUNT":             reflect.ValueOf(syscall.QTMOUNT),
		"QTTMP":               reflect.ValueOf(syscall.QTTMP),
		"RFCENVG":             reflect.ValueOf(syscall.RFCENVG),
		"RFCFDG":              reflect.ValueOf(syscall.RFCFDG),
		"RFCNAMEG":            reflect.ValueOf(syscall.RFCNAMEG),
		"RFENVG":              reflect.ValueOf(syscall.RFENVG),
		"RFFDG":               reflect.ValueOf(syscall.RFFDG),
		"RFMEM":               reflect.ValueOf(syscall.RFMEM),
		"RFNAMEG":             reflect.ValueOf(syscall.RFNAMEG),
		"RFNOMNT":             reflect.ValueOf(syscall.RFNOMNT),
		"RFNOTEG":             reflect.ValueOf(syscall.RFNOTEG),
		"RFNOWAIT":            reflect.ValueOf(syscall.RFNOWAIT),
		"RFPROC":              reflect.ValueOf(syscall.RFPROC),
		"RFREND":              reflect.ValueOf(syscall.RFREND),
		"RawSyscall":          reflect.ValueOf(syscall.RawSyscall),
		"RawSyscall6":         reflect.ValueOf(syscall.RawSyscall6),
		"Read":                reflect.ValueOf(syscall.Read),
		"Remove":              reflect.ValueOf(syscall.Remove),
		"SIGABRT":             reflect.ValueOf(syscall.SIGABRT),
		"SIGALRM":             reflect.ValueOf(syscall.SIGALRM),
		"SIGHUP":              reflect.ValueOf(syscall.SIGHUP),
		"SIGINT":              reflect.ValueOf(syscall.SIGINT),
		"SIGKILL":             reflect.ValueOf(syscall.SIGKILL),
		"SIGTERM":             reflect.ValueOf(syscall.SIGTERM),
		"STATFIXLEN":          reflect.ValueOf(syscall.STATFIXLEN),
		"STATMAX":             reflect.ValueOf(syscall.STATMAX),
		"SYS_ALARM":           reflect.ValueOf(syscall.SYS_ALARM),
		"SYS_AWAIT":           reflect.ValueOf(syscall.SYS_AWAIT),
		"SYS_BIND":            reflect.ValueOf(syscall.SYS_BIND),
		"SYS_BRK_":            reflect.ValueOf(syscall.SYS_BRK_),
		"SYS_CHDIR":           reflect.ValueOf(syscall.SYS_CHDIR),
		"SYS_CLOSE":           reflect.ValueOf(syscall.SYS_CLOSE),
		"SYS_CREATE":          reflect.ValueOf(syscall.SYS_CREATE),
		"SYS_DUP":             reflect.ValueOf(syscall.SYS_DUP),
		"SYS_ERRSTR":          reflect.ValueOf(syscall.SYS_ERRSTR),
		"SYS_EXEC":            reflect.ValueOf(syscall.SYS_EXEC),
		"SYS_EXITS":           reflect.ValueOf(syscall.SYS_EXITS),
		"SYS_FAUTH":           reflect.ValueOf(syscall.SYS_FAUTH),
		"SYS_FD2PATH":         reflect.ValueOf(syscall.SYS_FD2PATH),
		"SYS_FSTAT":           reflect.ValueOf(syscall.SYS_FSTAT),
		"SYS_FVERSION":        reflect.ValueOf(syscall.SYS_FVERSION),
		"SYS_FWSTAT":          reflect.ValueOf(syscall.SYS_FWSTAT),
		"SYS_MOUNT":           reflect.ValueOf(syscall.SYS_MOUNT),
		"SYS_NOTED":           reflect.ValueOf(syscall.SYS_NOTED),
		"SYS_NOTIFY":          reflect.ValueOf(syscall.SYS_NOTIFY),
		"SYS_NSEC":            reflect.ValueOf(syscall.SYS_NSEC),
		"SYS_OPEN":            reflect.ValueOf(syscall.SYS_OPEN),
		"SYS_OSEEK":           reflect.ValueOf(syscall.SYS_OSEEK),
		"SYS_PIPE":            reflect.ValueOf(syscall.SYS_PIPE),
		"SYS_PREAD":           reflect.ValueOf(syscall.SYS_PREAD),
		"SYS_PWRITE":          reflect.ValueOf(syscall.SYS_PWRITE),
		"SYS_REMOVE":          reflect.ValueOf(syscall.SYS_REMOVE),
		"SYS_RENDEZVOUS":      reflect.ValueOf(syscall.SYS_RENDEZVOUS),
		"SYS_RFORK":           reflect.ValueOf(syscall.SYS_RFORK),
		"SYS_SEEK":            reflect.ValueOf(syscall.SYS_SEEK),
		"SYS_SEGATTACH":       reflect.ValueOf(syscall.SYS_SEGATTACH),
		"SYS_SEGBRK":          reflect.ValueOf(syscall.SYS_SEGBRK),
		"SYS_SEGDETACH":       reflect.ValueOf(syscall.SYS_SEGDETACH),
		"SYS_SEGFLUSH":        reflect.ValueOf(syscall.SYS_SEGFLUSH),
		"SYS_SEGFREE":         reflect.ValueOf(syscall.SYS_SEGFREE),
		"SYS_SEMACQUIRE":      reflect.ValueOf(syscall.SYS_SEMACQUIRE),
		"SYS_SEMRELEASE":      reflect.ValueOf(syscall.SYS_SEMRELEASE),
		"SYS_SLEEP":           reflect.ValueOf(syscall.SYS_SLEEP),
		"SYS_STAT":            reflect.ValueOf(syscall.SYS_STAT),
		"SYS_SYSR1":           reflect.ValueOf(syscall.SYS_SYSR1),
		"SYS_TSEMACQUIRE":     reflect.ValueOf(syscall.SYS_TSEMACQUIRE),
		"SYS_UNMOUNT":         reflect.ValueOf(syscall.SYS_UNMOUNT),
		"SYS_WSTAT":           reflect.ValueOf(syscall.SYS_WSTAT),
		"S_IFBLK":             reflect.ValueOf(syscall.S_IFBLK),
		"S_IFCHR":             reflect.ValueOf(syscall.S_IFCHR),
		"S_IFDIR":             reflect.ValueOf(syscall.S_IFDIR),
		"S_IFIFO":             reflect.ValueOf(syscall.S_IFIFO),
		"S_IFLNK":             reflect.ValueOf(syscall.S_IFLNK),
		"S_IFMT":              reflect.ValueOf(syscall.S_IFMT),
		"S_IFREG":             reflect.ValueOf(syscall.S_IFREG),
		"S_IFSOCK":            reflect.ValueOf(syscall.S_IFSOCK),
		"Seek":                reflect.ValueOf(syscall.Seek),
		"Setenv":              reflect.ValueOf(syscall.Setenv),
		"SlicePtrFromStrings": reflect.ValueOf(syscall.SlicePtrFromStrings),
		"SocketDisableIPv6":   reflect.ValueOf(&syscall.SocketDisableIPv6).Elem(),
		"StartProcess":        reflect.ValueOf(syscall.StartProcess),
		"Stat":                reflect.ValueOf(syscall.Stat),
		"Stderr":              reflect.ValueOf(&syscall.Stderr).Elem(),
		"Stdin":               reflect.ValueOf(&syscall.Stdin).Elem(),
		"Stdout":              reflect.ValueOf(&syscall.Stdout).Elem(),
		"StringBytePtr":       reflect.ValueOf(syscall.StringBytePtr),
		"StringByteSlice":     reflect.ValueOf(syscall.StringByteSlice),
		"StringSlicePtr":      reflect.ValueOf(syscall.StringSlicePtr),
		"Syscall":             reflect.ValueOf(syscall.Syscall),
		"Syscall6":            reflect.ValueOf(syscall.Syscall6),
		"UnmarshalDir":        reflect.ValueOf(syscall.UnmarshalDir),
		"Unmount":             reflect.ValueOf(syscall.Unmount),
		"Unsetenv":            reflect.ValueOf(syscall.Unsetenv),
		"WaitProcess":         reflect.ValueOf(syscall.WaitProcess),
		"Write":               reflect.ValueOf(syscall.Write),
		"Wstat":               reflect.ValueOf(syscall.Wstat),

		// type definitions
		"Conn":        reflect.ValueOf((*syscall.Conn)(nil)),
		"Dir":         reflect.ValueOf((*syscall.Dir)(nil)),
		"ErrorString": reflect.ValueOf((*syscall.ErrorString)(nil)),
		"Note":        reflect.ValueOf((*syscall.Note)(nil)),
		"ProcAttr":    reflect.ValueOf((*syscall.ProcAttr)(nil)),
		"Qid":         reflect.ValueOf((*syscall.Qid)(nil)),
		"RawConn":     reflect.ValueOf((*syscall.RawConn)(nil)),
		"SysProcAttr": reflect.ValueOf((*syscall.SysProcAttr)(nil)),
		"Timespec":    reflect.ValueOf((*syscall.Timespec)(nil)),
		"Timeval":     reflect.ValueOf((*syscall.Timeval)(nil)),
		"Waitmsg":     reflect.ValueOf((*syscall.Waitmsg)(nil)),

		// interface wrapper definitions
		"_Conn":    reflect.ValueOf((*_syscall_Conn)(nil)),
		"_RawConn": reflect.ValueOf((*_syscall_RawConn)(nil)),
	}
}

// _syscall_Conn is an interface wrapper for Conn type
type _syscall_Conn struct {
	WSyscallConn func() (syscall.RawConn, error)
}

func (W _syscall_Conn) SyscallConn() (syscall.RawConn, error) { return W.WSyscallConn() }

// _syscall_RawConn is an interface wrapper for RawConn type
type _syscall_RawConn struct {
	WControl func(f func(fd uintptr)) error
	WRead    func(f func(fd uintptr) (done bool)) error
	WWrite   func(f func(fd uintptr) (done bool)) error
}

func (W _syscall_RawConn) Control(f func(fd uintptr)) error           { return W.WControl(f) }
func (W _syscall_RawConn) Read(f func(fd uintptr) (done bool)) error  { return W.WRead(f) }
func (W _syscall_RawConn) Write(f func(fd uintptr) (done bool)) error { return W.WWrite(f) }
