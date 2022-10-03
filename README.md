# OpsGenie on-call experiments

## Usage

```sh
oncall
```

## Install

1. Clone the repository

   ```sh
   git clone git@github.com:vmorsell/oncall.git
   ```

2. Compile and install the binary.

   ```sh
   cd oncall && go install
   ```

3. Set up the config file as described below.

## Config

Oncall is looking for a config file at `~/.config/oncall/config.yml`.

### Example

```yaml
opsGenie:
  apiKey: xyz
teamNames:
  - 'Team 1'
  - 'Team 2'
```
