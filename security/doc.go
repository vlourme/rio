// Package security
// 使用 transport.Reader 和 transport.Writer
// 开放任何与 tls 相关的内容，支持注册 cipher suite。
// Conn 移到 rio。
// 使用 tls.Config（尽可能接口化来支持GMSM），有些本来私有的，进行公有化。
// 提供：
// Record、Handshake、Close notify、
package security
