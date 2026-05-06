import pytest
from unittest.mock import MagicMock, patch
from pathlib import Path
from aetox.tools.loader import load_tools, create_default_registry, ToolRegistry
from aetox.tools.base import BaseTool

class TestLoader:
    def test_create_default_registry(self):
        registry = create_default_registry()
        assert isinstance(registry, ToolRegistry)
        # Should have loaded some tools if the directory is valid
        assert len(registry.get_all()) > 0

    def test_load_tools_skip_system_files(self):
        mock_registry = MagicMock()
        # Mocking Path.glob to return system files
        with patch.object(Path, "glob") as mock_glob:
            mock_glob.return_value = [
                Path("aetox/tools/__init__.py"),
                Path("aetox/tools/base.py"),
                Path("aetox/tools/registry.py"),
                Path("aetox/tools/loader.py")
            ]
            with patch("importlib.import_module") as mock_import:
                load_tools(mock_registry)
                mock_import.assert_not_called()

    def test_load_tools_import_error(self, caplog):
        mock_registry = MagicMock()
        with patch.object(Path, "glob") as mock_glob:
            mock_glob.return_value = [Path("aetox/tools/broken_tool.py")]
            with patch("importlib.import_module") as mock_import:
                # Only raise error for the specific tool module
                def side_effect(name):
                    if name == "aetox.tools.broken_tool":
                        raise Exception("Import Failed")
                    return MagicMock()
                mock_import.side_effect = side_effect
                
                load_tools(mock_registry)
                # Check if error message is in logs
                assert "โหลด tool ล้มเหลว" in caplog.text
                assert "Import Failed" in caplog.text

    def test_load_tools_success(self):
        mock_registry = MagicMock()
        
        # Create a mock module with a valid Tool class
        class ValidTool(BaseTool):
            def __init__(self):
                super().__init__(name="valid_tool", description="valid desc")
            def execute(self, params): return {}

        mock_module = MagicMock()
        mock_module.ValidTool = ValidTool
        
        with patch.object(Path, "glob") as mock_glob:
            mock_glob.return_value = [Path("aetox/tools/valid_tool.py")]
            with patch("importlib.import_module") as mock_import:
                mock_import.return_value = mock_module
                # Mock dir(module) to return our ValidTool
                with patch("aetox.tools.loader.dir") as mock_dir:
                    mock_dir.return_value = ["ValidTool"]
                    load_tools(mock_registry)
                    assert mock_registry.register.called
                    instance = mock_registry.register.call_args[0][0]
                    assert isinstance(instance, ValidTool)
