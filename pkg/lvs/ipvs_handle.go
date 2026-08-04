package lvs

// IPVSHandle abstracts IPVS kernel operations, allowing platform-specific implementations.
// On Linux, it wraps the real moby/ipvs netlink handle.
// On non-Linux systems, it provides a fake in-memory implementation for development and testing.
type IPVSHandle interface {
	Close()
	NewService(svc *Service) error
	UpdateService(svc *Service) error
	DelService(svc *Service) error
	GetServices() ([]*Service, error)
	NewDestination(svc *Service, dst *Destination) error
	UpdateDestination(svc *Service, dst *Destination) error
	DelDestination(svc *Service, dst *Destination) error
	GetDestinations(svc *Service) ([]*Destination, error)
	Flush() error
}
