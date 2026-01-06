"""gVisor sandbox backend for secure code execution."""

import asyncio
import tempfile
import uuid
from pathlib import Path

import docker
import structlog

from xatu_mcp.sandbox.base import ExecutionResult, SandboxBackend

logger = structlog.get_logger()


class GVisorBackend(SandboxBackend):
    """gVisor-based sandbox backend.

    Uses Docker with the gVisor runtime (runsc) for enhanced isolation.
    gVisor provides a user-space kernel that intercepts system calls,
    providing significantly stronger isolation than standard containers.

    This is the recommended backend for production deployments on Linux.
    Note: gVisor only works on Linux hosts.
    """

    RUNTIME = "runsc"

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
        self._active_containers: set[str] = set()
        self._runtime_checked = False

    @property
    def client(self) -> docker.DockerClient:
        """Get or create Docker client."""
        if self._client is None:
            self._client = docker.from_env()
        return self._client

    @property
    def name(self) -> str:
        return "gvisor"

    def _check_runtime(self) -> None:
        """Check if gVisor runtime is available."""
        if self._runtime_checked:
            return

        try:
            info = self.client.info()
            runtimes = info.get("Runtimes", {})
            if self.RUNTIME not in runtimes:
                available = list(runtimes.keys())
                raise RuntimeError(
                    f"gVisor runtime '{self.RUNTIME}' not found. "
                    f"Available runtimes: {available}. "
                    "Install gVisor: https://gvisor.dev/docs/user_guide/install/"
                )
            self._runtime_checked = True
            logger.info("gVisor runtime verified", runtime=self.RUNTIME)
        except docker.errors.APIError as e:
            raise RuntimeError(f"Failed to check Docker runtimes: {e}")

    async def execute(
        self,
        code: str,
        env: dict[str, str] | None = None,
        timeout: int | None = None,
    ) -> ExecutionResult:
        """Execute Python code in a gVisor-isolated container.

        Args:
            code: Python code to execute.
            env: Additional environment variables.
            timeout: Override default timeout.

        Returns:
            ExecutionResult with stdout, stderr, exit code, and output files.
        """
        # Check runtime availability on first execution
        self._check_runtime()

        execution_timeout = timeout or self.timeout
        execution_id = str(uuid.uuid4())[:8]

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
                "Starting gVisor container",
                execution_id=execution_id,
                image=self.image,
                timeout=execution_timeout,
                runtime=self.RUNTIME,
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
                    timeout=execution_timeout + 5,
                )
            except asyncio.TimeoutError:
                logger.warning(
                    "gVisor container execution timed out",
                    execution_id=execution_id,
                )
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
        """Run the container synchronously with gVisor runtime."""
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
                runtime=self.RUNTIME,  # Use gVisor runtime
                remove=False,
                detach=True,
                stderr=True,
                stdout=True,
            )

            self._active_containers.add(container.id)

            # Wait for container to finish
            result = container.wait(timeout=timeout)
            exit_code = result.get("StatusCode", 1)

            # Get logs
            stdout = container.logs(stdout=True, stderr=False).decode("utf-8", errors="replace")
            stderr = container.logs(stdout=False, stderr=True).decode("utf-8", errors="replace")

            duration = time.time() - start_time

            logger.debug(
                "gVisor container finished",
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
            logger.error("gVisor container error", execution_id=execution_id, error=str(e))
            return {
                "stdout": "",
                "stderr": f"Container error: {e}",
                "exit_code": 1,
                "duration": duration,
            }

        finally:
            if container:
                try:
                    container.remove(force=True)
                    self._active_containers.discard(container.id)
                except Exception as e:
                    logger.warning(
                        "Failed to remove container",
                        execution_id=execution_id,
                        error=str(e),
                    )

    async def cleanup(self) -> None:
        """Clean up any active containers."""
        for container_id in list(self._active_containers):
            try:
                container = self.client.containers.get(container_id)
                container.remove(force=True)
                logger.debug("Cleaned up container", container_id=container_id)
            except docker.errors.NotFound:
                pass
            except Exception as e:
                logger.warning("Failed to cleanup container", container_id=container_id, error=str(e))
            self._active_containers.discard(container_id)

        if self._client:
            self._client.close()
            self._client = None
