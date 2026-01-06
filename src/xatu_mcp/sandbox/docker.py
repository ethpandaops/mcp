"""Docker sandbox backend for code execution."""

import asyncio
import tempfile
import threading
import uuid
from pathlib import Path

import docker
import structlog

from xatu_mcp.sandbox.base import ExecutionResult, SandboxBackend

logger = structlog.get_logger()

# Execution ID prefix length for unique container identification
EXECUTION_ID_LENGTH = 8


class DockerBackend(SandboxBackend):
    """Docker-based sandbox backend.

    Uses standard Docker containers for code execution. Works on all platforms
    including macOS and Windows. Provides process-level isolation but shares
    the kernel with the host.

    This is the recommended backend for local development.
    """

    def __init__(
        self,
        image: str,
        timeout: int,
        memory_limit: str,
        cpu_limit: float,
        network: str,
    ) -> None:
        super().__init__(image, timeout, memory_limit, cpu_limit, network)
        self._client: docker.DockerClient | None = None
        self._active_containers: dict[str, docker.models.containers.Container] = {}
        self._lock = threading.Lock()  # Thread-safe access to _active_containers

    @property
    def client(self) -> docker.DockerClient:
        """Get or create Docker client."""
        if self._client is None:
            self._client = docker.from_env()
        return self._client

    @property
    def name(self) -> str:
        return "docker"

    def _track_container(self, execution_id: str, container: docker.models.containers.Container) -> None:
        """Thread-safe container tracking."""
        with self._lock:
            self._active_containers[execution_id] = container

    def _untrack_container(self, execution_id: str) -> docker.models.containers.Container | None:
        """Thread-safe container untracking. Returns the container if it was tracked."""
        with self._lock:
            return self._active_containers.pop(execution_id, None)

    def _force_kill_container(self, execution_id: str) -> None:
        """Force kill a container by execution_id (used on timeout)."""
        container = self._untrack_container(execution_id)
        if container:
            try:
                logger.warning("Force killing timed out container", execution_id=execution_id)
                container.kill()
                container.remove(force=True)
            except docker.errors.NotFound:
                pass  # Already removed
            except Exception as e:
                logger.error("Failed to force kill container", execution_id=execution_id, error=str(e))

    async def execute(
        self,
        code: str,
        env: dict[str, str] | None = None,
        timeout: int | None = None,
    ) -> ExecutionResult:
        """Execute Python code in a Docker container.

        Args:
            code: Python code to execute.
            env: Additional environment variables.
            timeout: Override default timeout.

        Returns:
            ExecutionResult with stdout, stderr, exit code, and output files.
        """
        execution_timeout = timeout or self.timeout
        execution_id = str(uuid.uuid4())[:EXECUTION_ID_LENGTH]

        # Create temp directories for shared files and output
        with tempfile.TemporaryDirectory() as tmpdir:
            tmpdir_path = Path(tmpdir)
            shared_dir = tmpdir_path / "shared"
            output_dir = tmpdir_path / "output"
            shared_dir.mkdir()
            output_dir.mkdir()

            # Write the code to a file
            script_path = shared_dir / "script.py"
            script_path.write_text(code)

            # Build environment variables
            container_env = env.copy() if env else {}

            # Build volume mounts
            volumes = {
                str(shared_dir): {"bind": "/shared", "mode": "ro"},
                str(output_dir): {"bind": "/output", "mode": "rw"},
            }

            logger.debug(
                "Starting container",
                execution_id=execution_id,
                image=self.image,
                timeout=execution_timeout,
            )

            try:
                # Run container in a thread pool to not block the event loop
                result = await asyncio.wait_for(
                    asyncio.get_event_loop().run_in_executor(
                        None,
                        lambda: self._run_container(
                            execution_id,
                            volumes,
                            container_env,
                            execution_timeout,
                        ),
                    ),
                    timeout=execution_timeout + 5,  # Extra time for container overhead
                )
            except asyncio.TimeoutError:
                # Force kill the container that's still running
                self._force_kill_container(execution_id)
                logger.warning("Container execution timed out", execution_id=execution_id)
                raise TimeoutError(f"Execution timed out after {execution_timeout}s")

            # Collect output files
            output_files = []
            for f in output_dir.iterdir():
                if f.is_file() and not f.name.startswith("."):
                    output_files.append(f.name)

            # Read metrics if present
            metrics = {}
            metrics_file = output_dir / ".metrics.json"
            if metrics_file.exists():
                import json

                try:
                    metrics = json.loads(metrics_file.read_text())
                except json.JSONDecodeError:
                    logger.warning("Failed to parse metrics file", execution_id=execution_id)

            return ExecutionResult(
                stdout=result["stdout"],
                stderr=result["stderr"],
                exit_code=result["exit_code"],
                output_files=output_files,
                metrics=metrics,
                duration_seconds=result["duration"],
            )

    def _run_container(
        self,
        execution_id: str,
        volumes: dict,
        env: dict[str, str],
        timeout: int,
    ) -> dict:
        """Run the container synchronously (called from thread pool).

        Includes security hardening options:
        - Non-root user (if supported by image)
        - Read-only root filesystem (except for /output)
        - No new privileges
        - Drop all capabilities
        - Disable network if not needed
        """
        import time

        start_time = time.time()
        container = None

        try:
            container = self.client.containers.run(
                self.image,
                command=["python", "/shared/script.py"],
                volumes=volumes,
                environment=env,
                network=self.network,
                mem_limit=self.memory_limit,
                cpu_period=100000,
                cpu_quota=int(100000 * self.cpu_limit),
                remove=False,  # We'll remove after getting logs
                detach=True,
                stderr=True,
                stdout=True,
                # Security hardening options
                user="nobody",  # Run as non-root user
                read_only=True,  # Read-only root filesystem
                security_opt=["no-new-privileges:true"],  # Prevent privilege escalation
                cap_drop=["ALL"],  # Drop all Linux capabilities
                tmpfs={"/tmp": "size=100M,mode=1777"},  # Writable /tmp in memory
                pids_limit=100,  # Limit number of processes
            )

            # Track container for potential timeout cleanup
            self._track_container(execution_id, container)

            # Wait for container to finish
            result = container.wait(timeout=timeout)
            exit_code = result.get("StatusCode", 1)

            # Get logs
            stdout = container.logs(stdout=True, stderr=False).decode("utf-8", errors="replace")
            stderr = container.logs(stdout=False, stderr=True).decode("utf-8", errors="replace")

            duration = time.time() - start_time

            logger.debug(
                "Container finished",
                execution_id=execution_id,
                exit_code=exit_code,
                duration=duration,
            )

            return {
                "stdout": stdout,
                "stderr": stderr,
                "exit_code": exit_code,
                "duration": duration,
            }

        except docker.errors.ContainerError as e:
            duration = time.time() - start_time
            return {
                "stdout": "",
                "stderr": str(e),
                "exit_code": e.exit_status,
                "duration": duration,
            }

        except Exception as e:
            duration = time.time() - start_time
            logger.error("Container error", execution_id=execution_id, error=str(e))
            return {
                "stdout": "",
                "stderr": f"Container error: {e}",
                "exit_code": 1,
                "duration": duration,
            }

        finally:
            # Untrack and remove container
            self._untrack_container(execution_id)
            if container:
                try:
                    container.remove(force=True)
                except docker.errors.NotFound:
                    pass  # Already removed (e.g., by force kill on timeout)
                except Exception as e:
                    logger.warning(
                        "Failed to remove container",
                        execution_id=execution_id,
                        error=str(e),
                    )

    async def cleanup(self) -> None:
        """Clean up any active containers."""
        with self._lock:
            containers_to_cleanup = list(self._active_containers.items())
            self._active_containers.clear()

        for execution_id, container in containers_to_cleanup:
            try:
                container.kill()
                container.remove(force=True)
                logger.debug("Cleaned up container", execution_id=execution_id)
            except docker.errors.NotFound:
                pass  # Already removed
            except Exception as e:
                logger.warning(
                    "Failed to cleanup container",
                    execution_id=execution_id,
                    error=str(e),
                )

        if self._client:
            self._client.close()
            self._client = None
