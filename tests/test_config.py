import unittest
import os
from aetox.core.config_loader import ConfigLoader

class TestConfigLoader(unittest.TestCase):
    def setUp(self):
        self.loader = ConfigLoader()

    def test_singleton(self):
        loader2 = ConfigLoader()
        self.assertEqual(self.loader, loader2)

    def test_get_model(self):
        model = self.loader.get_model("main")
        self.assertIsInstance(model, str)
        self.assertTrue(len(model) > 0)

    def test_memory_config(self):
        mem_cfg = self.loader.get_memory_config()
        self.assertIn("max_context_tokens", mem_cfg)
        self.assertIn("chunk_size", mem_cfg)

if __name__ == "__main__":
    unittest.main()
