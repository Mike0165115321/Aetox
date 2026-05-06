import pytest
from unittest.mock import MagicMock
from aetox.core.factory import AgentFactory
from aetox.agents.executor import ExecutorAgent
from aetox.agents.critic import CriticAgent
from aetox.planner.agent import AetoxPlanner

class TestAgentFactory:
    @pytest.fixture
    def mock_client(self):
        return MagicMock()

    @pytest.fixture
    def mock_engine(self):
        return MagicMock()

    @pytest.fixture
    def factory(self, mock_client, mock_engine):
        return AgentFactory(client=mock_client, engine=mock_engine)

    def test_factory_initialization(self, factory, mock_client, mock_engine):
        assert factory.shared_client == mock_client
        assert factory.shared_engine == mock_engine

    def test_create_executor(self, factory, mock_client, mock_engine):
        executor = factory.create_executor()
        assert isinstance(executor, ExecutorAgent)
        assert executor.client == mock_client
        assert executor.engine == mock_engine

    def test_create_critic(self, factory, mock_client, mock_engine):
        critic = factory.create_critic()
        assert isinstance(critic, CriticAgent)
        assert critic.client == mock_client
        assert critic.engine == mock_engine

    def test_create_planner(self, factory, mock_client, mock_engine):
        planner = factory.create_planner()
        assert isinstance(planner, AetoxPlanner)
        assert planner.client == mock_client
        assert planner.engine == mock_engine
