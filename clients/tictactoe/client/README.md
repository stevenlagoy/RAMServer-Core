# Tic-Tac-Toe Client

Tic-Tac-Toe client is implemented by an LLM.

The creation team should use prompt engineering to attempt a zero-shot implementation, followed by at least two rounds of evaluation and improvement. The LLM should be given access only to documentation concerning the protocol and message convention, and should not be allowed to see existing client implementations for any game, server source code, or testing files.

The analysis team should examine the created client implementation to determine whether the client is compliant with RAMServer-Protocol. Errors made in the implementation should be noted. The analysis team should perform initial analysis before examining the conversation / agent session which produced the implementation, but should continue analysis after this examination. The goal is to evaluate the protocol and the server's ability to interact with a client made by a non-developer.

The implementation must include: a ruleset in Go implementing the Game interface; a compliant client in another language which communicates with the server.

Select a language other than Go for the client implementation: prioritize interpretability and remember that another team has to evaluate the code's performance and compliance with the protocol. Strong typing and imperitive style is suggested to make analysis easier. The selected language should have some built-in multithreading behavior and the ability to provide networking.

Also be sure to use an LLM which will require minimal human overview when doing agentic coding, apart from any necessary commands being run and setting up the environment in the first place. You should select an up-to-date frontier model and use a high-effort setting, but not allow web access (we don't want the LLM to find our repo on GitHub or something like that).

Remember that the goal is not to evaluate the LLM's capabilities, but to explore whether the protocol, interfaces, and documentation are sufficient for an external developer to work with the RAMServer system.
