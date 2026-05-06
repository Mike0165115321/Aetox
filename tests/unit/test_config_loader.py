import pytest
from aetox.core.config_loader import ConfigLoader

class TestConfigLoader:
    @pytest.fixture
    def loader(self):
        return ConfigLoader()

    def test_singleton(self, loader):
        loader2 = ConfigLoader()
        assert loader is loader2

    def test_get_model(self, loader):
        model = loader.get_model("main")
        assert isinstance(model, str)
        assert len(model) > 0

    def test_session_config(self, loader):
        session_cfg = loader.get_session_config()
        assert isinstance(session_cfg, dict)
        assert "chat_history_limit" in session_cfg
        assert "history_truncate_chars" in session_cfg

    def test_get_options(self, loader):
        options = loader.get_options("main")
        assert isinstance(options, dict)
        assert "temperature" in options
