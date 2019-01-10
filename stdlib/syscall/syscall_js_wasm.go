package syscall

// Code generated by 'goexports syscall'. DO NOT EDIT.

import (
	"reflect"
	"syscall"
)

func init() {
	Value["syscall"] = map[string]reflect.Value{
		"AF_INET":             reflect.ValueOf(syscall.AF_INET),
		"AF_INET6":            reflect.ValueOf(syscall.AF_INET6),
		"AF_UNIX":             reflect.ValueOf(syscall.AF_UNIX),
		"AF_UNSPEC":           reflect.ValueOf(syscall.AF_UNSPEC),
		"Accept":              reflect.ValueOf(syscall.Accept),
		"Bind":                reflect.ValueOf(syscall.Bind),
		"BytePtrFromString":   reflect.ValueOf(syscall.BytePtrFromString),
		"ByteSliceFromString": reflect.ValueOf(syscall.ByteSliceFromString),
		"Chdir":               reflect.ValueOf(syscall.Chdir),
		"Chmod":               reflect.ValueOf(syscall.Chmod),
		"Chown":               reflect.ValueOf(syscall.Chown),
		"Clearenv":            reflect.ValueOf(syscall.Clearenv),
		"Close":               reflect.ValueOf(syscall.Close),
		"CloseOnExec":         reflect.ValueOf(syscall.CloseOnExec),
		"Connect":             reflect.ValueOf(syscall.Connect),
		"Dup":                 reflect.ValueOf(syscall.Dup),
		"Dup2":                reflect.ValueOf(syscall.Dup2),
		"E2BIG":               reflect.ValueOf(syscall.E2BIG),
		"EACCES":              reflect.ValueOf(syscall.EACCES),
		"EADDRINUSE":          reflect.ValueOf(syscall.EADDRINUSE),
		"EADDRNOTAVAIL":       reflect.ValueOf(syscall.EADDRNOTAVAIL),
		"EADV":                reflect.ValueOf(syscall.EADV),
		"EAFNOSUPPORT":        reflect.ValueOf(syscall.EAFNOSUPPORT),
		"EAGAIN":              reflect.ValueOf(syscall.EAGAIN),
		"EALREADY":            reflect.ValueOf(syscall.EALREADY),
		"EBADE":               reflect.ValueOf(syscall.EBADE),
		"EBADF":               reflect.ValueOf(syscall.EBADF),
		"EBADFD":              reflect.ValueOf(syscall.EBADFD),
		"EBADMSG":             reflect.ValueOf(syscall.EBADMSG),
		"EBADR":               reflect.ValueOf(syscall.EBADR),
		"EBADRQC":             reflect.ValueOf(syscall.EBADRQC),
		"EBADSLT":             reflect.ValueOf(syscall.EBADSLT),
		"EBFONT":              reflect.ValueOf(syscall.EBFONT),
		"EBUSY":               reflect.ValueOf(syscall.EBUSY),
		"ECANCELED":           reflect.ValueOf(syscall.ECANCELED),
		"ECASECLASH":          reflect.ValueOf(syscall.ECASECLASH),
		"ECHILD":              reflect.ValueOf(syscall.ECHILD),
		"ECHRNG":              reflect.ValueOf(syscall.ECHRNG),
		"ECOMM":               reflect.ValueOf(syscall.ECOMM),
		"ECONNABORTED":        reflect.ValueOf(syscall.ECONNABORTED),
		"ECONNREFUSED":        reflect.ValueOf(syscall.ECONNREFUSED),
		"ECONNRESET":          reflect.ValueOf(syscall.ECONNRESET),
		"EDEADLK":             reflect.ValueOf(syscall.EDEADLK),
		"EDEADLOCK":           reflect.ValueOf(syscall.EDEADLOCK),
		"EDESTADDRREQ":        reflect.ValueOf(syscall.EDESTADDRREQ),
		"EDOM":                reflect.ValueOf(syscall.EDOM),
		"EDOTDOT":             reflect.ValueOf(syscall.EDOTDOT),
		"EDQUOT":              reflect.ValueOf(syscall.EDQUOT),
		"EEXIST":              reflect.ValueOf(syscall.EEXIST),
		"EFAULT":              reflect.ValueOf(syscall.EFAULT),
		"EFBIG":               reflect.ValueOf(syscall.EFBIG),
		"EFTYPE":              reflect.ValueOf(syscall.EFTYPE),
		"EHOSTDOWN":           reflect.ValueOf(syscall.EHOSTDOWN),
		"EHOSTUNREACH":        reflect.ValueOf(syscall.EHOSTUNREACH),
		"EIDRM":               reflect.ValueOf(syscall.EIDRM),
		"EILSEQ":              reflect.ValueOf(syscall.EILSEQ),
		"EINPROGRESS":         reflect.ValueOf(syscall.EINPROGRESS),
		"EINTR":               reflect.ValueOf(syscall.EINTR),
		"EINVAL":              reflect.ValueOf(syscall.EINVAL),
		"EIO":                 reflect.ValueOf(syscall.EIO),
		"EISCONN":             reflect.ValueOf(syscall.EISCONN),
		"EISDIR":              reflect.ValueOf(syscall.EISDIR),
		"EL2HLT":              reflect.ValueOf(syscall.EL2HLT),
		"EL2NSYNC":            reflect.ValueOf(syscall.EL2NSYNC),
		"EL3HLT":              reflect.ValueOf(syscall.EL3HLT),
		"EL3RST":              reflect.ValueOf(syscall.EL3RST),
		"ELBIN":               reflect.ValueOf(syscall.ELBIN),
		"ELIBACC":             reflect.ValueOf(syscall.ELIBACC),
		"ELIBBAD":             reflect.ValueOf(syscall.ELIBBAD),
		"ELIBEXEC":            reflect.ValueOf(syscall.ELIBEXEC),
		"ELIBMAX":             reflect.ValueOf(syscall.ELIBMAX),
		"ELIBSCN":             reflect.ValueOf(syscall.ELIBSCN),
		"ELNRNG":              reflect.ValueOf(syscall.ELNRNG),
		"ELOOP":               reflect.ValueOf(syscall.ELOOP),
		"EMFILE":              reflect.ValueOf(syscall.EMFILE),
		"EMLINK":              reflect.ValueOf(syscall.EMLINK),
		"EMSGSIZE":            reflect.ValueOf(syscall.EMSGSIZE),
		"EMULTIHOP":           reflect.ValueOf(syscall.EMULTIHOP),
		"ENAMETOOLONG":        reflect.ValueOf(syscall.ENAMETOOLONG),
		"ENETDOWN":            reflect.ValueOf(syscall.ENETDOWN),
		"ENETRESET":           reflect.ValueOf(syscall.ENETRESET),
		"ENETUNREACH":         reflect.ValueOf(syscall.ENETUNREACH),
		"ENFILE":              reflect.ValueOf(syscall.ENFILE),
		"ENMFILE":             reflect.ValueOf(syscall.ENMFILE),
		"ENOANO":              reflect.ValueOf(syscall.ENOANO),
		"ENOBUFS":             reflect.ValueOf(syscall.ENOBUFS),
		"ENOCSI":              reflect.ValueOf(syscall.ENOCSI),
		"ENODATA":             reflect.ValueOf(syscall.ENODATA),
		"ENODEV":              reflect.ValueOf(syscall.ENODEV),
		"ENOENT":              reflect.ValueOf(syscall.ENOENT),
		"ENOEXEC":             reflect.ValueOf(syscall.ENOEXEC),
		"ENOLCK":              reflect.ValueOf(syscall.ENOLCK),
		"ENOLINK":             reflect.ValueOf(syscall.ENOLINK),
		"ENOMEDIUM":           reflect.ValueOf(syscall.ENOMEDIUM),
		"ENOMEM":              reflect.ValueOf(syscall.ENOMEM),
		"ENOMSG":              reflect.ValueOf(syscall.ENOMSG),
		"ENONET":              reflect.ValueOf(syscall.ENONET),
		"ENOPKG":              reflect.ValueOf(syscall.ENOPKG),
		"ENOPROTOOPT":         reflect.ValueOf(syscall.ENOPROTOOPT),
		"ENOSHARE":            reflect.ValueOf(syscall.ENOSHARE),
		"ENOSPC":              reflect.ValueOf(syscall.ENOSPC),
		"ENOSR":               reflect.ValueOf(syscall.ENOSR),
		"ENOSTR":              reflect.ValueOf(syscall.ENOSTR),
		"ENOSYS":              reflect.ValueOf(syscall.ENOSYS),
		"ENOTCONN":            reflect.ValueOf(syscall.ENOTCONN),
		"ENOTDIR":             reflect.ValueOf(syscall.ENOTDIR),
		"ENOTEMPTY":           reflect.ValueOf(syscall.ENOTEMPTY),
		"ENOTSOCK":            reflect.ValueOf(syscall.ENOTSOCK),
		"ENOTSUP":             reflect.ValueOf(syscall.ENOTSUP),
		"ENOTTY":              reflect.ValueOf(syscall.ENOTTY),
		"ENOTUNIQ":            reflect.ValueOf(syscall.ENOTUNIQ),
		"ENXIO":               reflect.ValueOf(syscall.ENXIO),
		"EOPNOTSUPP":          reflect.ValueOf(syscall.EOPNOTSUPP),
		"EOVERFLOW":           reflect.ValueOf(syscall.EOVERFLOW),
		"EPERM":               reflect.ValueOf(syscall.EPERM),
		"EPFNOSUPPORT":        reflect.ValueOf(syscall.EPFNOSUPPORT),
		"EPIPE":               reflect.ValueOf(syscall.EPIPE),
		"EPROCLIM":            reflect.ValueOf(syscall.EPROCLIM),
		"EPROTO":              reflect.ValueOf(syscall.EPROTO),
		"EPROTONOSUPPORT":     reflect.ValueOf(syscall.EPROTONOSUPPORT),
		"EPROTOTYPE":          reflect.ValueOf(syscall.EPROTOTYPE),
		"ERANGE":              reflect.ValueOf(syscall.ERANGE),
		"EREMCHG":             reflect.ValueOf(syscall.EREMCHG),
		"EREMOTE":             reflect.ValueOf(syscall.EREMOTE),
		"EROFS":               reflect.ValueOf(syscall.EROFS),
		"ESHUTDOWN":           reflect.ValueOf(syscall.ESHUTDOWN),
		"ESOCKTNOSUPPORT":     reflect.ValueOf(syscall.ESOCKTNOSUPPORT),
		"ESPIPE":              reflect.ValueOf(syscall.ESPIPE),
		"ESRCH":               reflect.ValueOf(syscall.ESRCH),
		"ESRMNT":              reflect.ValueOf(syscall.ESRMNT),
		"ESTALE":              reflect.ValueOf(syscall.ESTALE),
		"ETIME":               reflect.ValueOf(syscall.ETIME),
		"ETIMEDOUT":           reflect.ValueOf(syscall.ETIMEDOUT),
		"ETOOMANYREFS":        reflect.ValueOf(syscall.ETOOMANYREFS),
		"EUNATCH":             reflect.ValueOf(syscall.EUNATCH),
		"EUSERS":              reflect.ValueOf(syscall.EUSERS),
		"EWOULDBLOCK":         reflect.ValueOf(syscall.EWOULDBLOCK),
		"EXDEV":               reflect.ValueOf(syscall.EXDEV),
		"EXFULL":              reflect.ValueOf(syscall.EXFULL),
		"Environ":             reflect.ValueOf(syscall.Environ),
		"Exit":                reflect.ValueOf(syscall.Exit),
		"F_CNVT":              reflect.ValueOf(syscall.F_CNVT),
		"F_DUPFD":             reflect.ValueOf(syscall.F_DUPFD),
		"F_DUPFD_CLOEXEC":     reflect.ValueOf(syscall.F_DUPFD_CLOEXEC),
		"F_GETFD":             reflect.ValueOf(syscall.F_GETFD),
		"F_GETFL":             reflect.ValueOf(syscall.F_GETFL),
		"F_GETLK":             reflect.ValueOf(syscall.F_GETLK),
		"F_GETOWN":            reflect.ValueOf(syscall.F_GETOWN),
		"F_RDLCK":             reflect.ValueOf(syscall.F_RDLCK),
		"F_RGETLK":            reflect.ValueOf(syscall.F_RGETLK),
		"F_RSETLK":            reflect.ValueOf(syscall.F_RSETLK),
		"F_RSETLKW":           reflect.ValueOf(syscall.F_RSETLKW),
		"F_SETFD":             reflect.ValueOf(syscall.F_SETFD),
		"F_SETFL":             reflect.ValueOf(syscall.F_SETFL),
		"F_SETLK":             reflect.ValueOf(syscall.F_SETLK),
		"F_SETLKW":            reflect.ValueOf(syscall.F_SETLKW),
		"F_SETOWN":            reflect.ValueOf(syscall.F_SETOWN),
		"F_UNLCK":             reflect.ValueOf(syscall.F_UNLCK),
		"F_UNLKSYS":           reflect.ValueOf(syscall.F_UNLKSYS),
		"F_WRLCK":             reflect.ValueOf(syscall.F_WRLCK),
		"Fchdir":              reflect.ValueOf(syscall.Fchdir),
		"Fchmod":              reflect.ValueOf(syscall.Fchmod),
		"Fchown":              reflect.ValueOf(syscall.Fchown),
		"ForkLock":            reflect.ValueOf(syscall.ForkLock),
		"Fstat":               reflect.ValueOf(syscall.Fstat),
		"Fsync":               reflect.ValueOf(syscall.Fsync),
		"Ftruncate":           reflect.ValueOf(syscall.Ftruncate),
		"Getcwd":              reflect.ValueOf(syscall.Getcwd),
		"Getegid":             reflect.ValueOf(syscall.Getegid),
		"Getenv":              reflect.ValueOf(syscall.Getenv),
		"Geteuid":             reflect.ValueOf(syscall.Geteuid),
		"Getgid":              reflect.ValueOf(syscall.Getgid),
		"Getgroups":           reflect.ValueOf(syscall.Getgroups),
		"Getpagesize":         reflect.ValueOf(syscall.Getpagesize),
		"Getpid":              reflect.ValueOf(syscall.Getpid),
		"Getppid":             reflect.ValueOf(syscall.Getppid),
		"GetsockoptInt":       reflect.ValueOf(syscall.GetsockoptInt),
		"Gettimeofday":        reflect.ValueOf(syscall.Gettimeofday),
		"Getuid":              reflect.ValueOf(syscall.Getuid),
		"Getwd":               reflect.ValueOf(syscall.Getwd),
		"IPPROTO_IP":          reflect.ValueOf(syscall.IPPROTO_IP),
		"IPPROTO_IPV4":        reflect.ValueOf(syscall.IPPROTO_IPV4),
		"IPPROTO_IPV6":        reflect.ValueOf(syscall.IPPROTO_IPV6),
		"IPPROTO_TCP":         reflect.ValueOf(syscall.IPPROTO_TCP),
		"IPPROTO_UDP":         reflect.ValueOf(syscall.IPPROTO_UDP),
		"IPV6_V6ONLY":         reflect.ValueOf(syscall.IPV6_V6ONLY),
		"ImplementsGetwd":     reflect.ValueOf(syscall.ImplementsGetwd),
		"Kill":                reflect.ValueOf(syscall.Kill),
		"Lchown":              reflect.ValueOf(syscall.Lchown),
		"Link":                reflect.ValueOf(syscall.Link),
		"Listen":              reflect.ValueOf(syscall.Listen),
		"Lstat":               reflect.ValueOf(syscall.Lstat),
		"Mkdir":               reflect.ValueOf(syscall.Mkdir),
		"NsecToTimespec":      reflect.ValueOf(syscall.NsecToTimespec),
		"NsecToTimeval":       reflect.ValueOf(syscall.NsecToTimeval),
		"O_APPEND":            reflect.ValueOf(syscall.O_APPEND),
		"O_CLOEXEC":           reflect.ValueOf(syscall.O_CLOEXEC),
		"O_CREAT":             reflect.ValueOf(syscall.O_CREAT),
		"O_CREATE":            reflect.ValueOf(syscall.O_CREATE),
		"O_EXCL":              reflect.ValueOf(syscall.O_EXCL),
		"O_RDONLY":            reflect.ValueOf(syscall.O_RDONLY),
		"O_RDWR":              reflect.ValueOf(syscall.O_RDWR),
		"O_SYNC":              reflect.ValueOf(syscall.O_SYNC),
		"O_TRUNC":             reflect.ValueOf(syscall.O_TRUNC),
		"O_WRONLY":            reflect.ValueOf(syscall.O_WRONLY),
		"Open":                reflect.ValueOf(syscall.Open),
		"ParseDirent":         reflect.ValueOf(syscall.ParseDirent),
		"PathMax":             reflect.ValueOf(syscall.PathMax),
		"Pipe":                reflect.ValueOf(syscall.Pipe),
		"Pread":               reflect.ValueOf(syscall.Pread),
		"Pwrite":              reflect.ValueOf(syscall.Pwrite),
		"RawSyscall":          reflect.ValueOf(syscall.RawSyscall),
		"RawSyscall6":         reflect.ValueOf(syscall.RawSyscall6),
		"Read":                reflect.ValueOf(syscall.Read),
		"ReadDirent":          reflect.ValueOf(syscall.ReadDirent),
		"Readlink":            reflect.ValueOf(syscall.Readlink),
		"Recvfrom":            reflect.ValueOf(syscall.Recvfrom),
		"Recvmsg":             reflect.ValueOf(syscall.Recvmsg),
		"Rename":              reflect.ValueOf(syscall.Rename),
		"Rmdir":               reflect.ValueOf(syscall.Rmdir),
		"SIGCHLD":             reflect.ValueOf(syscall.SIGCHLD),
		"SIGINT":              reflect.ValueOf(syscall.SIGINT),
		"SIGKILL":             reflect.ValueOf(syscall.SIGKILL),
		"SIGQUIT":             reflect.ValueOf(syscall.SIGQUIT),
		"SIGTRAP":             reflect.ValueOf(syscall.SIGTRAP),
		"SOCK_DGRAM":          reflect.ValueOf(syscall.SOCK_DGRAM),
		"SOCK_RAW":            reflect.ValueOf(syscall.SOCK_RAW),
		"SOCK_SEQPACKET":      reflect.ValueOf(syscall.SOCK_SEQPACKET),
		"SOCK_STREAM":         reflect.ValueOf(syscall.SOCK_STREAM),
		"SOMAXCONN":           reflect.ValueOf(syscall.SOMAXCONN),
		"SO_ERROR":            reflect.ValueOf(syscall.SO_ERROR),
		"SYS_FCNTL":           reflect.ValueOf(syscall.SYS_FCNTL),
		"S_IEXEC":             reflect.ValueOf(syscall.S_IEXEC),
		"S_IFBLK":             reflect.ValueOf(syscall.S_IFBLK),
		"S_IFBOUNDSOCK":       reflect.ValueOf(syscall.S_IFBOUNDSOCK),
		"S_IFCHR":             reflect.ValueOf(syscall.S_IFCHR),
		"S_IFCOND":            reflect.ValueOf(syscall.S_IFCOND),
		"S_IFDIR":             reflect.ValueOf(syscall.S_IFDIR),
		"S_IFDSOCK":           reflect.ValueOf(syscall.S_IFDSOCK),
		"S_IFIFO":             reflect.ValueOf(syscall.S_IFIFO),
		"S_IFLNK":             reflect.ValueOf(syscall.S_IFLNK),
		"S_IFMT":              reflect.ValueOf(syscall.S_IFMT),
		"S_IFMUTEX":           reflect.ValueOf(syscall.S_IFMUTEX),
		"S_IFREG":             reflect.ValueOf(syscall.S_IFREG),
		"S_IFSEMA":            reflect.ValueOf(syscall.S_IFSEMA),
		"S_IFSHM":             reflect.ValueOf(syscall.S_IFSHM),
		"S_IFSHM_SYSV":        reflect.ValueOf(syscall.S_IFSHM_SYSV),
		"S_IFSOCK":            reflect.ValueOf(syscall.S_IFSOCK),
		"S_IFSOCKADDR":        reflect.ValueOf(syscall.S_IFSOCKADDR),
		"S_IREAD":             reflect.ValueOf(syscall.S_IREAD),
		"S_IRGRP":             reflect.ValueOf(syscall.S_IRGRP),
		"S_IROTH":             reflect.ValueOf(syscall.S_IROTH),
		"S_IRUSR":             reflect.ValueOf(syscall.S_IRUSR),
		"S_IRWXG":             reflect.ValueOf(syscall.S_IRWXG),
		"S_IRWXO":             reflect.ValueOf(syscall.S_IRWXO),
		"S_IRWXU":             reflect.ValueOf(syscall.S_IRWXU),
		"S_ISGID":             reflect.ValueOf(syscall.S_ISGID),
		"S_ISUID":             reflect.ValueOf(syscall.S_ISUID),
		"S_ISVTX":             reflect.ValueOf(syscall.S_ISVTX),
		"S_IWGRP":             reflect.ValueOf(syscall.S_IWGRP),
		"S_IWOTH":             reflect.ValueOf(syscall.S_IWOTH),
		"S_IWRITE":            reflect.ValueOf(syscall.S_IWRITE),
		"S_IWUSR":             reflect.ValueOf(syscall.S_IWUSR),
		"S_IXGRP":             reflect.ValueOf(syscall.S_IXGRP),
		"S_IXOTH":             reflect.ValueOf(syscall.S_IXOTH),
		"S_IXUSR":             reflect.ValueOf(syscall.S_IXUSR),
		"S_UNSUP":             reflect.ValueOf(syscall.S_UNSUP),
		"Seek":                reflect.ValueOf(syscall.Seek),
		"Sendfile":            reflect.ValueOf(syscall.Sendfile),
		"SendmsgN":            reflect.ValueOf(syscall.SendmsgN),
		"Sendto":              reflect.ValueOf(syscall.Sendto),
		"SetNonblock":         reflect.ValueOf(syscall.SetNonblock),
		"SetReadDeadline":     reflect.ValueOf(syscall.SetReadDeadline),
		"SetWriteDeadline":    reflect.ValueOf(syscall.SetWriteDeadline),
		"Setenv":              reflect.ValueOf(syscall.Setenv),
		"SetsockoptInt":       reflect.ValueOf(syscall.SetsockoptInt),
		"Shutdown":            reflect.ValueOf(syscall.Shutdown),
		"Socket":              reflect.ValueOf(syscall.Socket),
		"StartProcess":        reflect.ValueOf(syscall.StartProcess),
		"Stat":                reflect.ValueOf(syscall.Stat),
		"Stderr":              reflect.ValueOf(syscall.Stderr),
		"Stdin":               reflect.ValueOf(syscall.Stdin),
		"Stdout":              reflect.ValueOf(syscall.Stdout),
		"StopIO":              reflect.ValueOf(syscall.StopIO),
		"StringBytePtr":       reflect.ValueOf(syscall.StringBytePtr),
		"StringByteSlice":     reflect.ValueOf(syscall.StringByteSlice),
		"Symlink":             reflect.ValueOf(syscall.Symlink),
		"Syscall":             reflect.ValueOf(syscall.Syscall),
		"Syscall6":            reflect.ValueOf(syscall.Syscall6),
		"Sysctl":              reflect.ValueOf(syscall.Sysctl),
		"TimespecToNsec":      reflect.ValueOf(syscall.TimespecToNsec),
		"TimevalToNsec":       reflect.ValueOf(syscall.TimevalToNsec),
		"Truncate":            reflect.ValueOf(syscall.Truncate),
		"Unlink":              reflect.ValueOf(syscall.Unlink),
		"Unsetenv":            reflect.ValueOf(syscall.Unsetenv),
		"UtimesNano":          reflect.ValueOf(syscall.UtimesNano),
		"Wait4":               reflect.ValueOf(syscall.Wait4),
		"Write":               reflect.ValueOf(syscall.Write),
	}

	Type["syscall"] = map[string]reflect.Type{
		"Conn":          reflect.TypeOf((*syscall.Conn)(nil)).Elem(),
		"Dirent":        reflect.TypeOf((*syscall.Dirent)(nil)).Elem(),
		"Errno":         reflect.TypeOf((*syscall.Errno)(nil)).Elem(),
		"Iovec":         reflect.TypeOf((*syscall.Iovec)(nil)).Elem(),
		"ProcAttr":      reflect.TypeOf((*syscall.ProcAttr)(nil)).Elem(),
		"RawConn":       reflect.TypeOf((*syscall.RawConn)(nil)).Elem(),
		"Rusage":        reflect.TypeOf((*syscall.Rusage)(nil)).Elem(),
		"Signal":        reflect.TypeOf((*syscall.Signal)(nil)).Elem(),
		"Sockaddr":      reflect.TypeOf((*syscall.Sockaddr)(nil)).Elem(),
		"SockaddrInet4": reflect.TypeOf((*syscall.SockaddrInet4)(nil)).Elem(),
		"SockaddrInet6": reflect.TypeOf((*syscall.SockaddrInet6)(nil)).Elem(),
		"SockaddrUnix":  reflect.TypeOf((*syscall.SockaddrUnix)(nil)).Elem(),
		"Stat_t":        reflect.TypeOf((*syscall.Stat_t)(nil)).Elem(),
		"SysProcAttr":   reflect.TypeOf((*syscall.SysProcAttr)(nil)).Elem(),
		"Timespec":      reflect.TypeOf((*syscall.Timespec)(nil)).Elem(),
		"Timeval":       reflect.TypeOf((*syscall.Timeval)(nil)).Elem(),
		"WaitStatus":    reflect.TypeOf((*syscall.WaitStatus)(nil)).Elem(),
	}
}
