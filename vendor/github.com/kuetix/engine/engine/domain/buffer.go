package domain

type Buffer struct {
	Type       string
	Data       []byte
	DataSize   int
	BufferSize int
	Error      error
}

//goland:noinspection GoUnusedExportedFunction
func NewBuffer(bufferSize int) *Buffer {
	return &Buffer{
		Data:       make([]byte, bufferSize),
		BufferSize: bufferSize,
	}
}

func (b *Buffer) Reset() {
	for i := 0; i < b.DataSize; i++ {
		b.Data[i] = 0
	}
	b.DataSize = 0
	b.Error = nil
}
