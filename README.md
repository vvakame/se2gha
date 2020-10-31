# Slack Event 2 GitHub Actions

send slack event to github actions.
receive slack event, convert, post github [repository_dispatch](https://docs.github.com/en/free-pro-team@latest/developers/webhooks-and-events/webhook-events-and-payloads#repository_dispatch) event.
process something you want on github actions.

## Supported slack event

* `reaction_added`
    * send `slack-event-reaction_added-${reaction}` event to github

## Setup

3 assets required

* [Slack app](https://api.slack.com/apps)
    * Event Subscriptions
        * what you need. e.g. `reaction_added`
    * Scopes
        * `team:read`
        * `users.profile:read`
        * `channels:history`
        * `reactions:read`
    * Signing Secret → `SLACK_SIGNING_SECRET`
    * Access Token → `SLACK_ACCESS_TOKEN`
* [GitHub Personal Access Token](https://github.com/settings/tokens)
    * Scopes
        * `repo`
    * Personal Access Token → `GHA_REPO_TOKEN`
* Environment variables for app
    * `SLACK_SIGNING_SECRET`
    * `SLACK_ACCESS_TOKEN`
    * `GHA_REPO_TOKEN`
    * `GHA_REPOS`
        * `${RepositoryOwner}/${RepositioryName}` format. e.g. `vvakame/se2gha`
        * if you wanna send a event to multiple repositories, you can use `,` to delimiter

## Example use case

* [create issue by slack reaction added](https://github.com/vvakame/se2gha/blob/master/.github/workflows/issue-from-slack.yml)
