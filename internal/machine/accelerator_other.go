//go:build !darwin

package machine

func observeAccelerator() (string, error) {
	// QEMU remains an engineering adapter on non-Darwin Machines. Whether it
	// receives hardware acceleration is not a v0.1 product support claim.
	return "qemu", nil
}
