package vm

// msgDontwait: package syscall has no MSG_DONTWAIT on aix, so Recvfrom of
// the subset takes MSG_PEEK only there (checkRecvfrom).
const msgDontwait = 0
