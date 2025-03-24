package tagged

type Pointer[E any] uint64

// minTagBits is the minimum number of tag bits that we expect.
const minTagBits = 10
