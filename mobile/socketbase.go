package mobile

type ProtectSocket interface {
	Protect(filedescriptor int) int
}
