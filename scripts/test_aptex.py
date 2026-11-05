"""Check APTEX lifecycle isolation without stopping live services."""

import importlib.util
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("aptex_launcher", Path(__file__).with_name("aptex.py"))
aptex = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(aptex)


class AptexLauncherTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="aptex client ")
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.repo = self.directory / "backend checkout"
        self.home = self.directory / "home"
        self.repo.mkdir()
        self.home.mkdir()
        for name in ("docker-compose.yml", "docker-compose.host-model.yml", ".env.example"):
            (self.repo / name).write_text("")
        (self.repo / "web" / "public").mkdir(parents=True)
        (self.repo / "web" / "public" / "afterglow.svg").write_text("<svg/>")
        root_patch = patch.object(aptex, "ROOT", self.repo)
        home_patch = patch.object(Path, "home", return_value=self.home)
        root_patch.start()
        home_patch.start()
        self.addCleanup(root_patch.stop)
        self.addCleanup(home_patch.stop)

    def test_prepare_preserves_settings_and_uses_existing_local_model(self):
        models = self.directory / "model store"
        (models / "blobs").mkdir(parents=True)
        (models / "manifests").mkdir()
        (self.repo / ".env").write_text("PERSONAL_SETTING=retained\nSCOPEWEAVER_LOCAL_LLM_ONLY=false\n")
        with patch.dict(aptex.os.environ, {"OLLAMA_MODELS_DIR": str(models)}):
            settings = aptex.prepare_backend(self.repo)
        self.assertEqual(settings["PERSONAL_SETTING"], "retained")
        self.assertEqual(settings["SCOPEWEAVER_LOCAL_LLM_ONLY"], "true")
        self.assertEqual(settings["SCOPEWEAVER_LLM_MODEL"], "qwen3:32b")
        self.assertEqual(settings["OLLAMA_MODELS_DIR"], str(models))
        self.assertTrue(settings["POSTGRES_PASSWORD"])
        self.assertEqual((self.repo / ".env").stat().st_mode & 0o777, 0o600)

    def test_start_reuses_running_services_and_leaves_odysseus_configuration_alone(self):
        odysseus = self.directory / "odysseus"
        odysseus.mkdir()
        config = odysseus / ".env"
        config.write_text("APP_PORT=7860\nPERSONAL_SETTING=retained\n")
        original = config.read_bytes()
        with patch.object(aptex, "prepare_backend", return_value={"SCOPEWEAVER_LLM_MODEL": "qwen3:32b"}), \
             patch.object(aptex, "ensure_docker"), \
             patch.object(aptex, "all_backend_services_running", return_value=True), \
             patch.object(aptex, "model_directory", return_value=self.directory / "models"), \
             patch.object(aptex, "ollama_ip_and_mount", return_value="172.20.0.2"), \
             patch.object(aptex, "wait_for_local_model"), \
             patch.object(aptex, "update_local_allowlist", return_value=False), \
             patch.object(aptex, "wait_for_backend"), \
             patch.object(aptex, "ensure_administrator"), \
             patch.object(aptex.subprocess, "run") as run:
            aptex.start()
        self.assertEqual(run.call_args.args[0], aptex.compose_command(
            self.repo, "up", "-d", "--no-build", "postgres", "ollama", "scopeweaver"))
        self.assertEqual(run.call_count, 1)
        self.assertEqual(config.read_bytes(), original)

    def test_stop_targets_only_aptex_services(self):
        with patch.object(aptex.shutil, "which", return_value=None), \
             patch.object(aptex, "docker_running", return_value=True), \
             patch.object(aptex.subprocess, "run") as run:
            aptex.stop()
        self.assertEqual(run.call_args.args[0], aptex.compose_command(
            self.repo, "stop", "scopeweaver", "ollama", "postgres"))
        self.assertEqual(run.call_count, 1)

    def test_docker_start_does_not_stop_other_projects(self):
        with patch.object(aptex, "docker_running", side_effect=[False, True]), \
             patch.object(aptex.shutil, "which", return_value="/usr/bin/fixture"), \
             patch.object(aptex.subprocess, "run", return_value=SimpleNamespace(returncode=0)) as run:
            aptex.ensure_docker()
        self.assertEqual(run.call_args.args[0], ["sudo", "-n", "systemctl", "start", "docker.service"])
        self.assertEqual(run.call_count, 1)

    def test_administrator_setup_creates_no_gateway_token(self):
        with patch.object(aptex, "request", side_effect=[
            (200, {"initialized": False, "username": "Afterglow"}),
            (200, {"token": "initial-administrator-token"}),
        ]) as request:
            aptex.ensure_administrator("http://127.0.0.1:8787")
        self.assertEqual([call.args[2] for call in request.call_args_list], ["/api/auth/status", "/api/auth/init"])
        credential = aptex.admin_password_file()
        self.assertEqual(credential.stat().st_mode & 0o777, 0o600)
        self.assertIn("Afterglow username: Afterglow", credential.read_text())
        with patch.object(aptex, "request", return_value=(200, {"initialized": True})) as request:
            aptex.ensure_administrator("http://127.0.0.1:8787")
        self.assertEqual(request.call_count, 1)

    def test_client_has_its_own_profile_and_does_not_control_servers(self):
        with patch.object(aptex, "request", return_value=(200, {"status": "ok"})), \
             patch.object(aptex.shutil, "which", return_value="/usr/bin/google-chrome"), \
             patch.object(aptex, "docker_running", side_effect=AssertionError("client started Docker")), \
             patch.object(aptex.subprocess, "Popen") as popen:
            aptex.client()
        command = popen.call_args.args[0]
        self.assertIn("--app=http://127.0.0.1:8787", command)
        self.assertIn("--class=APTEX", command)
        self.assertIn(f"--user-data-dir={self.home / '.local/share/aptex/client'}", command)
        self.assertEqual(popen.call_count, 1)

    def test_install_preserves_normal_odysseus_shortcuts(self):
        bin_dir = self.home / ".local" / "bin"
        bin_dir.mkdir(parents=True)
        normal = bin_dir / "odysseus-start"
        normal.write_text("original Odysseus command")
        legacy = self.directory / "odysseus" / "script" / "odysseus_cyber_stack.py"
        legacy.parent.mkdir(parents=True)
        legacy.write_text("")
        (bin_dir / "odysseus-cyber-start").symlink_to(legacy)
        aptex.install()
        self.assertEqual(normal.read_text(), "original Odysseus command")
        for name in ("aptex", "aptex-start", "aptex-stop", "odysseus-cyber-start"):
            self.assertEqual((bin_dir / name).resolve(), Path(aptex.__file__).resolve())
        desktop = (self.home / ".local/share/applications/aptex.desktop").read_text()
        self.assertIn("StartupWMClass=APTEX", desktop)
        self.assertIn('"' + str(Path(aptex.__file__).resolve()) + '" client', desktop)

    def test_invalid_port_fails_before_opening_any_client(self):
        (self.repo / ".env").write_text("ARTEX_PORT=99999\n")
        with self.assertRaisesRegex(ValueError, "valid TCP port"):
            aptex.base_url()


if __name__ == "__main__":
    unittest.main()
