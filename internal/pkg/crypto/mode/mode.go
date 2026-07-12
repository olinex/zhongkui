package mode

type Mode interface {
	Encrypt(p []byte) ([]byte, error)
	Decrypt(p []byte) ([]byte, error)
}
