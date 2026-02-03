# Firecracker Agent - Fase 2 Implementación Completa

Este documento describe la implementación de la Fase 2 del firecracker-agent, que incluye integración real con Firecracker para gestión de microVMs.

## Resumen de Implementación

### Componentes Nuevos

#### 1. **Cliente HTTP para Firecracker API** (`internal/firecracker/client.go`)

Cliente HTTP que se comunica con Firecracker vía Unix socket.

**Características:**
- Comunicación vía Unix socket con Firecracker API
- Métodos implementados:
  - `SetBootSource()` - Configura kernel y parámetros de boot
  - `SetMachineConfig()` - Configura vCPUs y memoria
  - `AddDrive()` - Agrega discos (rootfs)
  - `AddNetworkInterface()` - Configura interfaces de red
  - `StartInstance()` - Inicia la VM
  - `SendCtrlAltDel()` - Envía señal de shutdown graceful
  - `GetInstanceInfo()` - Obtiene información de la instancia

**Ejemplo de uso:**
```go
client := firecracker.NewClient("/path/to/firecracker.socket")
err := client.SetBootSource(ctx, BootSource{
    KernelImagePath: "/path/to/vmlinux",
    BootArgs: "console=ttyS0 reboot=k panic=1 pci=off",
})
```

#### 2. **Gestor de Procesos** (`internal/firecracker/process.go`)

Gestiona el ciclo de vida de procesos Firecracker.

**Características:**
- Inicia procesos Firecracker con parámetros correctos
- Espera a que el socket esté listo
- Manejo de logs a archivo
- Shutdown graceful (SIGTERM) y forzado (SIGKILL)
- Verificación de estado del proceso
- Cleanup automático de recursos

**Struct VMProcess:**
```go
type VMProcess struct {
    PID        int
    Cmd        *exec.Cmd
    SocketPath string
    LogFile    *os.File
    Client     *Client
    log        *logrus.Logger
}
```

**Métodos:**
- `StartFirecrackerProcess()` - Inicia Firecracker
- `Stop()` - Detiene gracefully
- `Kill()` - Termina forzadamente
- `IsRunning()` - Verifica si está ejecutándose

#### 3. **Módulo de Red** (`internal/network/manager.go`)

Gestiona configuración de red para VMs usando TAP devices y Linux bridge.

**Características:**
- Creación/eliminación de TAP devices
- Configuración de bridge Linux
- Generación determinística de direcciones MAC
- Configuración de iptables para NAT
- Validación y cleanup automático

**Métodos:**
```go
CreateTAPDevice(vmID string) (tapName string, error)
DeleteTAPDevice(tapName string) error
EnsureBridgeExists() error
GenerateMAC(vmID string) string
ConfigureIPTables(tapName, vmIP string) error
```

**Flujo de red:**
```
VM (eth0) <-> TAP device <-> Linux Bridge <-> Host eth0 (NAT)
```

#### 4. **Módulo de Storage** (`internal/storage/manager.go`)

Gestiona almacenamiento para VMs con soporte para overlay filesystem.

**Características:**
- Preparación de directorios para VMs
- Copy-on-write con qcow2 overlay
- Gestión de kernel y rootfs
- Cleanup automático de recursos

**Métodos:**
```go
PrepareVMStorage(vmID, kernelPath, rootfsPath string) (*VMStorage, error)
CleanupVMStorage(vmID string) error
EnsureVMsDir() error
```

**Estructura de directorios:**
```
/var/lib/firecracker/vms/
├── vm-12345/
│   ├── rootfs.ext4 (qcow2 overlay)
│   ├── vmlinux.bin (link o copia)
│   ├── firecracker.socket
│   ├── firecracker.log
│   ├── upper/ (overlay upperdir)
│   └── work/ (overlay workdir)
```

#### 5. **Manager Actualizado** (`internal/firecracker/manager.go`)

Manager completamente refactorizado con integración real de Firecracker.

**Flujo CreateVM:**

1. **Validación** - Verifica que la VM no exista
2. **Storage** - Prepara directorios, copia/linkea kernel y rootfs
3. **Network** - Crea TAP device y lo agrega al bridge
4. **Process** - Inicia proceso Firecracker
5. **Configuration** - Configura VM vía Firecracker API:
   - Boot source (kernel + boot args)
   - Machine config (vCPUs + memory)
   - Rootfs drive
   - Network interface (TAP + MAC)
6. **Start** - Inicia la VM
7. **Register** - Guarda en registro interno

**Flujo DeleteVM:**

1. **Stop Process** - Termina proceso Firecracker
2. **Network Cleanup** - Elimina TAP device
3. **Storage Cleanup** - Elimina directorio de VM
4. **Unregister** - Elimina del registro interno

## Configuración

### Archivo de Configuración (`configs/agent.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 50051

firecracker:
  binary_path: "/usr/bin/firecracker"
  jailer_path: "/usr/bin/jailer"
  kernel_path: "/var/lib/firecracker/images/vmlinux.bin"
  rootfs_path: "/var/lib/firecracker/images/rootfs.ext4"

network:
  bridge_name: "fcbr0"
  tap_prefix: "fctap"

storage:
  vms_dir: "/var/lib/firecracker/vms"
  use_overlay: true

monitoring:
  enabled: true
  metrics_port: 9090

log:
  level: "info"
  format: "json"
```

## Requisitos del Sistema

### Software

- **Firecracker** >= 1.0
- **Linux kernel** >= 4.14 (con KVM)
- **qemu-img** (para overlay filesystem)
- **iproute2** (comando `ip`)
- **iptables**

### Permisos

El firecracker-agent necesita:
- Permisos root o CAP_NET_ADMIN (para crear TAP devices y configurar bridge)
- Acceso a `/dev/kvm`
- Permisos de escritura en directorio de VMs

### Preparación del Sistema

#### 1. Instalar Firecracker

```bash
# Descargar Firecracker
wget https://github.com/firecracker-microvm/firecracker/releases/download/v1.6.0/firecracker-v1.6.0-x86_64.tgz
tar -xzf firecracker-v1.6.0-x86_64.tgz

# Copiar binarios
sudo cp release-v1.6.0-x86_64/firecracker-v1.6.0-x86_64 /usr/bin/firecracker
sudo chmod +x /usr/bin/firecracker
```

#### 2. Preparar Kernel y Rootfs

```bash
# Crear directorios
sudo mkdir -p /var/lib/firecracker/images
sudo mkdir -p /var/lib/firecracker/vms

# Descargar kernel (ejemplo)
wget https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin
sudo mv vmlinux.bin /var/lib/firecracker/images/

# Crear o descargar rootfs (ejemplo con Ubuntu)
wget https://cloud-images.ubuntu.com/minimal/releases/jammy/release/ubuntu-22.04-minimal-cloudimg-amd64-root.tar.xz
# Convertir a ext4 si es necesario
```

#### 3. Configurar Red

```bash
# Crear bridge
sudo ip link add name fcbr0 type bridge
sudo ip link set fcbr0 up

# Asignar IP al bridge (opcional)
sudo ip addr add 10.0.0.1/24 dev fcbr0

# Habilitar IP forwarding
sudo sysctl -w net.ipv4.ip_forward=1

# Configurar NAT
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
```

## Cómo Usar

### 1. Compilar

```bash
cd firecracker-agent
go build -o bin/fc-agent ./cmd/fc-agent
```

### 2. Configurar

Editar `configs/agent.yaml` con las rutas correctas.

### 3. Ejecutar

```bash
# Como root (necesario para TAP devices)
sudo ./bin/fc-agent --config configs/agent.yaml
```

### 4. Probar con mikrom-go

```bash
# Terminal 1: firecracker-agent
sudo ./bin/fc-agent --config configs/agent.yaml

# Terminal 2: mikrom-go API
cd mikrom-go
go run cmd/api/main.go

# Terminal 3: mikrom-go worker
go run cmd/worker/main.go

# Terminal 4: Crear VM
curl -X POST http://localhost:8080/api/v1/vms \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-vm",
    "vcpu_count": 2,
    "memory_mb": 512
  }'
```

## Limitaciones Conocidas

1. **Firecracker no soporta pause/resume**: No se puede pausar una VM
2. **Firecracker no soporta restart**: Las VMs deben recrearse completamente
3. **Requiere root**: Necesario para gestionar TAP devices
4. **Solo x86_64**: Firecracker solo soporta arquitectura x86_64
5. **Linux only**: Firecracker solo funciona en Linux

## Troubleshooting

### Error: "Permission denied" al crear TAP device

**Solución:**
```bash
# Ejecutar como root
sudo ./bin/fc-agent --config configs/agent.yaml

# O agregar capacidades
sudo setcap cap_net_admin+ep ./bin/fc-agent
```

### Error: "Cannot find Firecracker binary"

**Solución:**
```bash
# Verificar que existe
which firecracker

# Actualizar path en configs/agent.yaml
firecracker:
  binary_path: "/path/to/firecracker"
```

### Error: "Failed to start Firecracker process"

**Solución:**
```bash
# Verificar KVM
ls -l /dev/kvm
# Debe ser: crw-rw---- 1 root kvm

# Agregar usuario al grupo kvm
sudo usermod -a -G kvm $USER

# Reloguearse
```

### Error: "Bridge not found"

**Solución:**
```bash
# Crear bridge manualmente
sudo ip link add name fcbr0 type bridge
sudo ip link set fcbr0 up

# O dejar que el agent lo cree automáticamente
```

### VMs no tienen conectividad

**Solución:**
```bash
# Verificar IP forwarding
sysctl net.ipv4.ip_forward

# Habilitar si está en 0
sudo sysctl -w net.ipv4.ip_forward=1

# Verificar iptables
sudo iptables -t nat -L -n -v

# Agregar regla MASQUERADE si falta
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
```

## Próximos Pasos

### Fase 3 (Mejoras Futuras)

1. **Jailer Integration** - Usar jailer de Firecracker para seguridad
2. **Snapshots** - Soporte para snapshots de VMs
3. **Hot-plug** - Soporte para hot-plug de dispositivos
4. **Metrics** - Métricas más detalladas por VM
5. **TLS/mTLS** - Comunicación segura con mikrom-go
6. **Rate Limiting** - Límites de recursos por usuario
7. **Multi-host** - Gestión de múltiples hosts

## Arquitectura Completa

```
┌─────────────────────────────────────────────────────────────┐
│                         mikrom-go                           │
│  ┌──────────┐      ┌──────────┐      ┌──────────┐         │
│  │   API    │ ───> │  Worker  │ ───> │  gRPC    │         │
│  │  (REST)  │      │ (asynq)  │      │  Client  │         │
│  └──────────┘      └──────────┘      └─────┬────┘         │
└─────────────────────────────────────────────┼──────────────┘
                                              │ gRPC
                                              │ (port 50051)
┌─────────────────────────────────────────────┼──────────────┐
│                    firecracker-agent        │              │
│  ┌────────────────────────────────────────┬─┴────┐         │
│  │             gRPC Server                 │      │         │
│  └────────────────────────────────────────┬─┬────┘         │
│                                           │ │               │
│  ┌──────────────────┐  ┌─────────────────┴─┴───────────┐  │
│  │ Network Manager  │  │   Firecracker Manager         │  │
│  │  - TAP devices   │  │   - VM lifecycle              │  │
│  │  - Bridge config │  │   - Process management        │  │
│  │  - iptables      │  │   - State tracking            │  │
│  └──────────────────┘  └──────────┬────────────────────┘  │
│                                    │                        │
│  ┌──────────────────┐  ┌──────────┴────────────────────┐  │
│  │ Storage Manager  │  │   Firecracker API Client      │  │
│  │  - Overlay FS    │  │   - Unix socket               │  │
│  │  - VM dirs       │  │   - HTTP client               │  │
│  └──────────────────┘  └──────────┬────────────────────┘  │
└─────────────────────────────────────┼──────────────────────┘
                                      │ Unix Socket
                                      │ /path/to/firecracker.socket
                                      ▼
                            ┌──────────────────┐
                            │   Firecracker    │
                            │   Process (KVM)  │
                            └──────────────────┘
```

## Testing

Para probar la implementación completa:

```bash
# 1. Verificar build
cd firecracker-agent
go build ./...
./bin/fc-agent --version

# 2. Verificar requisitos
firecracker --version
qemu-img --version
ip link show
cat /proc/sys/net/ipv4/ip_forward

# 3. Ejecutar agent (como root)
sudo ./bin/fc-agent --config configs/agent.yaml

# 4. En otra terminal, probar con grpcurl
grpcurl -plaintext localhost:50051 \
  firecracker.v1.FirecrackerAgent/HealthCheck
```

## Conclusión

La Fase 2 está completamente implementada con:
- ✅ Integración real con Firecracker API
- ✅ Gestión de procesos robusta
- ✅ Configuración de red con TAP/bridge
- ✅ Storage con overlay filesystem
- ✅ Cleanup automático de recursos
- ✅ Manejo de errores completo
- ✅ Logging estructurado

El sistema está listo para crear y gestionar microVMs reales de Firecracker a través de gRPC desde mikrom-go.
