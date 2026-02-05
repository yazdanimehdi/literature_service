# Phase 3B: LLM Integration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement LLM-based keyword extraction from natural language queries and paper abstracts, supporting OpenAI and Anthropic providers.

**Architecture:** A `KeywordExtractor` interface defines the extraction contract. Provider-specific clients (OpenAI, Anthropic) implement the interface using their respective APIs. A structured JSON prompt instructs the LLM to return keywords in a parseable format. A factory creates the appropriate provider based on config.

**Tech Stack:** Go 1.25, net/http, encoding/json, testify, httptest

---

## Task 1: Create KeywordExtractor Interface and Types
## Task 2: Implement OpenAI Provider
## Task 3: Implement Anthropic Provider
## Task 4: Implement Provider Factory
## Task 5: Run Full Test Suite
